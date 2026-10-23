package usecase_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type fakeCards struct {
	repo.ApplicationRepository
	app domain.Application
	err error
}

func (f fakeCards) Card(context.Context, uuid.UUID, *uuid.UUID) (domain.Application, error) {
	return f.app, f.err
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
	consent uuid.UUID,
) (json.RawMessage, error) {
	f.called = fmt.Sprintf("%s %s %s", src, inn, consent)
	return f.payload, f.err
}

type fakeChecks struct {
	saved []domain.CheckResult
}

func (f *fakeChecks) Add(_ context.Context, _ uuid.UUID, r domain.CheckResult) error {
	f.saved = append(f.saved, r)
	return nil
}

func TestCheckFetch(t *testing.T) {
	id := uuid.New()
	app := domain.Application{ID: id, Company: domain.Company{INN: "0000100012"}}
	down := fmt.Errorf("fns: %w", domain.ErrUnavailable)
	tests := []struct {
		name    string
		cardErr error
		srcErr  error
		want    domain.CheckStatus
		err     error
		saved   []domain.CheckResult
	}{
		{"ответил", nil, nil, domain.CheckOK, nil,
			[]domain.CheckResult{{Source: domain.SourceFNS, Status: domain.CheckOK, Payload: []byte(`{"a":1}`)}}},
		{"нет в источнике", nil, fmt.Errorf("fns: %w", domain.ErrNotFound), domain.CheckNotFound, nil,
			[]domain.CheckResult{{Source: domain.SourceFNS, Status: domain.CheckNotFound}}},
		// недоступность повторяет temporal, записывать рано
		{"источник лежит", nil, down, "", domain.ErrUnavailable, nil},
		{"нет заявки", domain.ErrNotFound, nil, "", domain.ErrNotFound, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeSources{payload: []byte(`{"a":1}`), err: tt.srcErr}
			checks := &fakeChecks{}
			uc := usecase.NewCheck(fakeCards{app: app, err: tt.cardErr}, checks, src)

			got, err := uc.Fetch(t.Context(), id, domain.SourceFNS)
			require.ErrorIs(t, err, tt.err)
			require.Equal(t, tt.want, got)
			require.Equal(t, tt.saved, checks.saved)
			if tt.cardErr == nil {
				require.Equal(t, "fns 0000100012 "+id.String(), src.called)
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
