package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type fakeFaults struct {
	list []domain.Fault
	set  []domain.Fault
}

func (f *fakeFaults) List(context.Context) ([]domain.Fault, error) { return f.list, nil }

func (f *fakeFaults) Set(_ context.Context, fault domain.Fault) error {
	f.set = append(f.set, fault)
	return nil
}

func TestFaults(t *testing.T) {
	f := &fakeFaults{list: []domain.Fault{
		{Source: domain.SourceBKI, Mode: "timeout"},
		// сбой на одну компанию пульт не показывает
		{Source: domain.SourceEGRUL, INN: "0000100012", Mode: "http_503"},
	}}
	uc := usecase.NewFault(f)

	got, err := uc.List(t.Context())
	require.NoError(t, err)
	require.Equal(t, []domain.Fault{
		{Source: domain.SourceEGRUL, Mode: "ok"},
		{Source: domain.SourceFNS, Mode: "ok"},
		{Source: domain.SourceBKI, Mode: "timeout"},
		{Source: domain.SourceInternal, Mode: "ok"},
	}, got)

	require.NoError(t, uc.Set(t.Context(), domain.SourceEGRUL, "flaky"))
	require.NoError(t, uc.Set(t.Context(), domain.SourceFNS, "http_503"))
	require.Equal(t, []domain.Fault{
		{Source: domain.SourceEGRUL, Mode: "flaky", Left: 3},
		{Source: domain.SourceFNS, Mode: "http_503"},
	}, f.set)
}
