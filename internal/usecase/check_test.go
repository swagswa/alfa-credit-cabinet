package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type fakeCards struct {
	repo.ApplicationRepository
	app     domain.Application
	err     error
	consent domain.BKIConsent
}

func (f fakeCards) Card(context.Context, uuid.UUID, *uuid.UUID) (domain.Application, error) {
	return f.app, f.err
}

func (f fakeCards) BKIConsent(context.Context, uuid.UUID) (domain.BKIConsent, error) {
	if f.consent.ID == uuid.Nil {
		return domain.BKIConsent{}, domain.ErrNotFound
	}
	return f.consent, nil
}

type fakeSources struct {
	payload json.RawMessage
	err     error
	called  string
}

func (f *fakeSources) Fetch(
	_ context.Context,
	src domain.Source,
	inn string,
	consent domain.BKIConsent,
) (json.RawMessage, error) {
	f.called = fmt.Sprintf("%s %s %s", src, inn, consent.ID)
	return f.payload, f.err
}

type fakeChecks struct {
	saved, failed []domain.CheckResult
}

func (f *fakeChecks) Add(_ context.Context, _ uuid.UUID, r domain.CheckResult) error {
	f.saved = append(f.saved, r)
	return nil
}

func (f *fakeChecks) Failed(_ context.Context, _ uuid.UUID, r domain.CheckResult) error {
	f.failed = append(f.failed, r)
	return nil
}

func TestCheckFetch(t *testing.T) {
	id := uuid.New()
	app := domain.Application{ID: id, Company: domain.Company{INN: "0000100012"}}
	consent := domain.BKIConsent{ID: uuid.New(), GivenAt: time.Now()}
	down := fmt.Errorf("fns: %w", domain.ErrUnavailable)
	try := domain.Attempt{N: 2, Of: 5}
	denied := fmt.Errorf("bki: %w", domain.ErrForbidden)
	noConsent := []domain.CheckResult{{Source: domain.SourceBKI, Status: domain.CheckUnavailable,
		Error: "бюро отказало: нет действующего согласия на запрос кредитной истории", Attempt: try}}
	fns := "fns 0000100012 " + uuid.Nil.String()
	tests := []struct {
		name    string
		src     domain.Source
		consent domain.BKIConsent
		cardErr error
		srcErr  error
		want    domain.CheckStatus
		err     error
		saved   []domain.CheckResult
		called  string
	}{
		{
			"ответил",
			domain.SourceFNS,
			consent,
			nil,
			nil,
			domain.CheckOK,
			nil,
			[]domain.CheckResult{
				{Source: domain.SourceFNS, Status: domain.CheckOK, Payload: []byte(`{"a":1}`), Attempt: try},
			},
			fns,
		},
		{
			"нет в источнике",
			domain.SourceFNS,
			consent,
			nil,
			fmt.Errorf("fns: %w", domain.ErrNotFound),
			domain.CheckNotFound,
			nil,
			[]domain.CheckResult{{Source: domain.SourceFNS, Status: domain.CheckNotFound, Attempt: try}},
			fns,
		},
		// недоступность повторяет temporal, записывать рано
		{"источник лежит", domain.SourceFNS, consent, nil, down, "", domain.ErrUnavailable, nil, fns},
		{"нет заявки", domain.SourceFNS, consent, domain.ErrNotFound, nil, "", domain.ErrNotFound, nil, ""},
		{
			"бки с согласием",
			domain.SourceBKI,
			consent,
			nil,
			nil,
			domain.CheckOK,
			nil,
			[]domain.CheckResult{
				{Source: domain.SourceBKI, Status: domain.CheckOK, Payload: []byte(`{"a":1}`), Attempt: try},
			},
			"bki 0000100012 " + consent.ID.String(),
		},
		// отказ бюро не повторяем и не роняем, это результат проверки с причиной
		{"бюро отказало", domain.SourceBKI, consent, nil, denied, domain.CheckUnavailable, nil, noConsent,
			"bki 0000100012 " + consent.ID.String()},
		{"записи согласия нет", domain.SourceBKI, domain.BKIConsent{}, nil, denied, domain.CheckUnavailable, nil,
			noConsent, "bki 0000100012 " + uuid.Nil.String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeSources{payload: []byte(`{"a":1}`), err: tt.srcErr}
			checks := &fakeChecks{}
			uc := usecase.NewCheck(fakeCards{app: app, err: tt.cardErr, consent: tt.consent}, checks, src)

			got, err := uc.Fetch(t.Context(), id, tt.src, try)
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.saved, checks.saved)
			// неудачную попытку видно на живой карте, хотя в базу она не пишется
			if errors.Is(tt.srcErr, domain.ErrUnavailable) {
				require.Equal(
					t,
					[]domain.CheckResult{{Source: tt.src, Error: tt.srcErr.Error(), Attempt: try}},
					checks.failed,
				)
			} else {
				require.Empty(t, checks.failed)
			}
			if tt.cardErr == nil {
				require.Equal(t, tt.called, src.called)
			} else {
				require.Empty(t, src.called, "без заявки в источник не ходим")
			}
		})
	}
}

func TestCheckSaveUnavailable(t *testing.T) {
	checks := &fakeChecks{}
	uc := usecase.NewCheck(fakeCards{}, checks, &fakeSources{})
	require.NoError(t, uc.SaveUnavailable(t.Context(), uuid.New(), domain.SourceBKI, "bki: 503"))
	require.Equal(t, []domain.CheckResult{
		{Source: domain.SourceBKI, Status: domain.CheckUnavailable, Error: "bki: 503"},
	}, checks.saved)
}
