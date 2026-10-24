package faults_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/faults"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
)

// против настоящего мока: формат ручки знают обе стороны, разъехаться им нельзя
func TestClient(t *testing.T) {
	srv := httptest.NewServer(mocks.New())
	defer srv.Close()
	c := faults.New(srv.URL, time.Second)

	require.NoError(t, c.Set(t.Context(), domain.Fault{Source: domain.SourceEGRUL, Mode: "flaky", Left: 3}))
	got, err := c.List(t.Context())
	require.NoError(t, err)
	require.Equal(t, []domain.Fault{{Source: domain.SourceEGRUL, Mode: "flaky", Left: 3}}, got)

	require.NoError(t, c.Set(t.Context(), domain.Fault{Source: domain.SourceEGRUL, Mode: domain.FaultOff}))
	got, err = c.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, got)

	require.ErrorIs(t, c.Set(t.Context(), domain.Fault{Source: "cbr", Mode: "ok"}), domain.ErrUnavailable)
	srv.Close()
	_, err = c.List(t.Context())
	require.ErrorIs(t, err, domain.ErrUnavailable)
}
