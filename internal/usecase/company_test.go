package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type fakeEGRUL struct {
	calls int
	err   error
	c     domain.Company
}

func (f *fakeEGRUL) Company(_ context.Context, inn string) (domain.Company, error) {
	f.calls++
	c := f.c
	c.INN = inn
	return c, f.err
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name      string
		inn       string
		sourceErr error
		wantErr   error
		wantCalls int
	}{
		{name: "юрлицо", inn: "0000100012", wantCalls: 1},
		{name: "ип", inn: "770123456703", wantCalls: 1},
		{name: "контрольная цифра не сходится", inn: "0000100013", wantErr: domain.ErrInvalid},
		{name: "пустой", inn: "", wantErr: domain.ErrInvalid},
		{name: "источник лежит", inn: "0000500010", sourceErr: domain.ErrUnavailable, wantErr: domain.ErrUnavailable,
			wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeEGRUL{err: tt.sourceErr}
			c, err := usecase.NewCompany(src).Lookup(t.Context(), tt.inn)
			require.Equal(t, tt.wantCalls, src.calls)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.inn, c.INN)
		})
	}
}
