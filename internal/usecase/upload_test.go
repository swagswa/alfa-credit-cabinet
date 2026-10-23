package usecase_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

var (
	pdf = []byte("%PDF-1.4\n1 0 obj\n")
	png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
)

// docs собирает usecase документов на базе appEnv, llm отдаёт выписку кофейни
func (e *appEnv) docs(ai bool, wf *fakeWorkflows) (usecase.DocumentUseCase, *fakeText) {
	text := &fakeText{}
	llm := &fakeLLM{values: domain.DocValues{Statement: statement(coffeeINN)}}
	apps := repo.NewApplication(db.New(e.pool))
	return usecase.NewDocument(repo.NewDocument(e.pool), apps, wf, text, llm, ai), text
}

// statement: год оборотов по 1 млн в месяц, всё сходится
func statement(inn string) *domain.Statement {
	s := &domain.Statement{INN: inn, PeriodFrom: "2025-10-01", PeriodTo: "2026-09-30", OpeningBalance: 50_000_00}
	for i := range 12 {
		m := time.Date(2025, time.October+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		s.MonthlyIn = append(s.MonthlyIn, domain.MonthAmount{Month: m.Format("2006-01"), Amount: 1_000_000_00})
		s.TurnoverIn += 1_000_000_00
	}
	s.TurnoverOut = s.TurnoverIn - 20_000_00
	s.ClosingBalance = s.OpeningBalance + 20_000_00
	return s
}

func TestUpload(t *testing.T) {
	e := newAppEnv(t)
	draft, submitted := e.app(domain.StatusDraft), e.app(domain.StatusSubmitted)
	big := append(bytes.Clone(pdf), make([]byte, domain.MaxDocSize)...)
	tests := []struct {
		name    string
		ai      bool
		wfErr   error
		actor   domain.Actor
		app     uuid.UUID
		kind    domain.DocKind
		file    []byte
		want    domain.DocStatus
		started bool
		wantErr error
	}{
		{name: "без ИИ сразу skipped", file: pdf, want: domain.DocSkipped},
		{name: "с ИИ на распознавание", ai: true, file: png, want: domain.DocRecognizing, started: true},
		{name: "паспорт не распознаём", ai: true, kind: "passport", file: pdf, want: domain.DocUploaded},
		{name: "temporal лежит, вводят руками", ai: true, wfErr: errors.New("unavailable"), file: pdf,
			want: domain.DocFailed, started: true},
		{name: "договор формирует банк", kind: domain.DocLoanAgreement, file: pdf, wantErr: domain.ErrInvalid},
		{name: "больше 10 МБ", file: big, wantErr: domain.ErrTooLarge},
		{name: "пустой файл", file: []byte{}, wantErr: domain.ErrInvalid},
		{name: "не pdf и не картинка", file: []byte("<html><body>выписка</body></html>"), wantErr: domain.ErrInvalid},
		{name: "чужая заявка", actor: e.user("logistics"), file: pdf, wantErr: domain.ErrNotFound},
		{name: "заявка уже подана", app: submitted, file: pdf, wantErr: domain.ErrIllegalTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &fakeWorkflows{err: tt.wfErr}
			uc, _ := e.docs(tt.ai, wf)
			actor := e.client
			if tt.actor.ID != uuid.Nil {
				actor = tt.actor
			}
			app := draft
			if tt.app != uuid.Nil {
				app = tt.app
			}
			kind := domain.DocStatement
			if tt.kind != "" {
				kind = tt.kind
			}

			d, err := uc.Upload(t.Context(), actor, app, kind, "vypiska.pdf", tt.file)
			require.ErrorIs(t, err, tt.wantErr)
			if err != nil {
				return
			}
			require.Equal(t, tt.want, d.Status)
			require.Equal(t, app, d.ApplicationID)
			require.Equal(t, len(tt.file), d.Size)
			if !tt.started {
				require.Nil(t, wf.args)
				return
			}
			require.Equal(t, []any{usecase.QueueDocs, "doc-" + d.ID.String(), "RecognizeWorkflow", d.ID}, wf.args)
			if tt.wfErr != nil {
				require.NotEmpty(t, d.Error, "клиенту нужно объяснить, почему вводить руками")
			}
		})
	}
}

func TestDocumentAccess(t *testing.T) {
	e := newAppEnv(t)
	uc, _ := e.docs(false, nil)
	d, err := uc.Upload(t.Context(), e.client, e.app(domain.StatusDraft), domain.DocStatement, "v.pdf", pdf)
	require.NoError(t, err)

	tests := []struct {
		login string
		err   error
	}{
		{"coffee", nil},
		{"logistics", domain.ErrNotFound},
		{"underwriter", nil},
	}
	for _, tt := range tests {
		t.Run(tt.login, func(t *testing.T) {
			a := e.user(tt.login)
			got, err := uc.Get(t.Context(), a, d.ID)
			require.ErrorIs(t, err, tt.err)
			if tt.err == nil {
				require.Equal(t, coffeeINN, got.INN)
			}
			f, err := uc.File(t.Context(), a, d.ID)
			require.ErrorIs(t, err, tt.err)
			if tt.err == nil {
				require.Equal(t, pdf, f.Content)
				require.Equal(t, "application/pdf", f.ContentType)
				require.Equal(t, "v.pdf", f.FileName)
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	e := newAppEnv(t)
	ctx := t.Context()
	app := e.app(domain.StatusDraft)

	t.Run("руками без ИИ", func(t *testing.T) {
		uc, _ := e.docs(false, nil)
		d, err := uc.Upload(ctx, e.client, app, domain.DocStatement, "v.pdf", pdf)
		require.NoError(t, err)

		v := domain.DocValues{Statement: statement(coffeeINN)}
		d, err = uc.Confirm(ctx, e.client, d.ID, v)
		require.NoError(t, err)
		require.Equal(t, domain.DocConfirmed, d.Status)
		require.Equal(t, &v, d.Confirmed)
		require.Empty(t, d.EditedFields, "распознанного не было, править нечего")
		require.NotNil(t, d.ConfirmedAt)

		_, err = uc.Confirm(ctx, e.client, d.ID, v)
		require.ErrorIs(t, err, domain.ErrIllegalTransition, "второе подтверждение")
	})

	t.Run("поправил распознанное", func(t *testing.T) {
		uc, _ := e.docs(true, &fakeWorkflows{})
		d, err := uc.Upload(ctx, e.client, app, domain.DocStatement, "v.pdf", pdf)
		require.NoError(t, err)
		require.NoError(t, uc.Recognize(ctx, d.ID))

		s := statement(coffeeINN)
		s.OpeningBalance += 100_00
		s.ClosingBalance += 100_00
		d, err = uc.Confirm(ctx, e.client, d.ID, domain.DocValues{Statement: s})
		require.NoError(t, err)
		require.Equal(t, []string{"closing_balance_kop", "opening_balance_kop"}, d.EditedFields)
		require.Equal(t, statement(coffeeINN), d.Extracted.Statement, "что прочитала модель, остаётся для аудита")
	})

	t.Run("повторное распознавание не перетирает", func(t *testing.T) {
		uc, text := e.docs(true, &fakeWorkflows{})
		d, err := uc.Upload(ctx, e.client, app, domain.DocStatement, "v.pdf", pdf)
		require.NoError(t, err)
		_, err = uc.Confirm(ctx, e.client, d.ID, domain.DocValues{Statement: statement(coffeeINN)})
		require.NoError(t, err)

		// activity стартовала раньше, чем клиент подтвердил, и дошла только сейчас
		require.NoError(t, uc.Recognize(ctx, d.ID))
		d, err = uc.Get(ctx, e.client, d.ID)
		require.NoError(t, err)
		require.Equal(t, domain.DocConfirmed, d.Status)
		require.Nil(t, d.Extracted)
		require.Zero(t, text.calls)
	})

	uc, _ := e.docs(false, nil)
	wrongINN := statement("0000300011")
	broken := statement(coffeeINN)
	broken.ClosingBalance = 0
	tests := []struct {
		name   string
		kind   domain.DocKind
		values domain.DocValues
		fields []string
	}{
		{name: "блок не под вид", kind: domain.DocStatement,
			values: domain.DocValues{Financials: &domain.Financials{INN: coffeeINN, Year: 2025}}},
		{name: "пусто", kind: domain.DocStatement},
		{name: "паспорт не подтверждают", kind: "passport", values: domain.DocValues{Statement: statement(coffeeINN)}},
		{name: "чужой ИНН", kind: domain.DocStatement, values: domain.DocValues{Statement: wrongINN},
			fields: []string{"inn"}},
		{name: "остаток не сходится", kind: domain.DocStatement, values: domain.DocValues{Statement: broken},
			fields: []string{"closing_balance_kop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := uc.Upload(ctx, e.client, app, tt.kind, "v.pdf", pdf)
			require.NoError(t, err)
			_, err = uc.Confirm(ctx, e.client, d.ID, tt.values)
			var p domain.FieldErrors
			if tt.fields == nil {
				require.ErrorIs(t, err, domain.ErrInvalid)
			} else {
				require.True(t, errors.As(err, &p), fmt.Sprint(err))
				var fields []string
				for _, f := range p {
					fields = append(fields, f.Field)
				}
				require.Equal(t, tt.fields, fields)
			}
		})
	}
}
