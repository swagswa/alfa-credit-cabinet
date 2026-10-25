package usecase_test

import (
	"cmp"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type fakeDocs struct {
	repo.DocumentRepository
	doc      domain.DocFile
	statuses []domain.DocStatus
	saved    *domain.Recognition
	reason   string
}

func (f *fakeDocs) File(context.Context, uuid.UUID, *uuid.UUID) (domain.DocFile, error) {
	return f.doc, nil
}

func (f *fakeDocs) SetStatus(_ context.Context, _ uuid.UUID, s domain.DocStatus, reason string) error {
	f.statuses, f.reason = append(f.statuses, s), reason
	return nil
}

func (f *fakeDocs) SaveRecognition(
	_ context.Context, _ uuid.UUID, s domain.DocStatus, rec *domain.Recognition, reason string,
) error {
	f.statuses, f.saved, f.reason = append(f.statuses, s), rec, reason
	return nil
}

type fakeText struct {
	calls int
	scan  bool
	err   error
}

func (f *fakeText) Text(context.Context, string, []byte) (string, domain.TextSource, error) {
	f.calls++
	if f.scan {
		return "Выписка", domain.TextOCR, f.err
	}
	return "Выписка", domain.TextPDF, f.err
}

func (f *fakeText) Reread(context.Context, string, []byte) (string, error) {
	f.calls++
	return "Выписка", nil
}

// второе прочтение отдаёт reread, если он задан
type fakeLLM struct {
	values domain.DocValues
	reread *domain.DocValues
	err    error
	calls  int
}

func (f *fakeLLM) Extract(context.Context, domain.DocKind, string) ([]byte, domain.DocValues, error) {
	f.calls++
	if f.calls > 1 && f.reread != nil {
		return []byte(`{"n":2}`), *f.reread, f.err
	}
	return []byte(`{"n":1}`), f.values, f.err
}

func (f *fakeLLM) Model() string                          { return "qwen3:4b-instruct-2507-q4_K_M" }
func (f *fakeLLM) Digest(context.Context) (string, error) { return "0edcdef34593", nil }

func TestRecognize(t *testing.T) {
	good := domain.DocValues{Financials: &domain.Financials{
		INN: "0000100012", Year: 2025, Revenue: 1000_00, BalanceTotal: 1000_00,
	}}
	wrongINN := domain.DocValues{Financials: &domain.Financials{INN: "0000300011", Year: 2025}}
	// OCR прочитал валюту баланса меньше выручки, второе прочтение верное
	misread := domain.DocValues{Financials: &domain.Financials{
		INN: "0000100012", Year: 2025, Revenue: 1000_00, BalanceTotal: 1000_00, LiabilitiesShort: 2000_00,
	}}
	down := errors.New("connection refused")
	tests := []struct {
		name     string
		status   domain.DocStatus
		kind     domain.DocKind
		disabled bool
		textErr  error
		scan     bool
		llm      fakeLLM
		want     []domain.DocStatus
		wantErr  error
		wantOCR  int
	}{
		{name: "распознали", llm: fakeLLM{values: good},
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocRecognized}, wantOCR: 1},
		{name: "ИИ выключен", disabled: true, want: []domain.DocStatus{domain.DocSkipped}},
		{name: "уже подтвердили руками", status: domain.DocConfirmed},
		{name: "паспорт не распознаём", kind: "passport"},
		{name: "чужой ИНН", llm: fakeLLM{values: wrongINN},
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocFailed}, wantOCR: 1},
		{name: "скан сошёлся со второго прочтения", scan: true, llm: fakeLLM{values: misread, reread: &good},
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocRecognized}, wantOCR: 2},
		{name: "скан не сошёлся и со второго", scan: true, llm: fakeLLM{values: wrongINN, reread: &wrongINN},
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocFailed}, wantOCR: 2},
		{name: "битый файл", textErr: domain.ErrInvalid,
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocFailed}, wantOCR: 1},
		{name: "модель ответила не по схеме", llm: fakeLLM{err: domain.ErrInvalid},
			want: []domain.DocStatus{domain.DocRecognizing, domain.DocFailed}, wantOCR: 1},
		{name: "ollama лежит, повторит temporal", llm: fakeLLM{err: down},
			want: []domain.DocStatus{domain.DocRecognizing}, wantErr: down, wantOCR: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := &fakeDocs{doc: domain.DocFile{
				Kind: cmp.Or(tt.kind, domain.DocFinancials), Status: cmp.Or(tt.status, domain.DocUploaded),
				INN: "0000100012",
			}}
			text := &fakeText{err: tt.textErr, scan: tt.scan}
			uc := usecase.NewDocument(docs, nil, nil, text, &tt.llm, !tt.disabled)

			require.ErrorIs(t, uc.Recognize(t.Context(), uuid.New()), tt.wantErr)
			require.Equal(t, tt.want, docs.statuses)
			require.Equal(t, tt.wantOCR, text.calls)
			if last := len(tt.want) - 1; last > 0 && tt.want[last] == domain.DocFailed {
				require.NotEmpty(t, docs.reason, "клиенту нужно объяснить, почему вводить руками")
				require.Equal(t, "qwen3:4b-instruct-2507-q4_K_M", docs.saved.Model)
			}
			if tt.wantOCR == 2 {
				require.JSONEq(t, `[{"n":1},{"n":2}]`, string(docs.saved.Raw), "в аудите оба ответа модели")
			}
		})
	}
}
