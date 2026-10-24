package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpanName(t *testing.T) {
	tests := map[string]string{
		"-- name: ApplicationCard :one\nSELECT id FROM applications": "ApplicationCard",
		"  BEGIN":  "BEGIN",
		"select 1": "select",
		"-- блокируем строку\nSELECT 1 FOR UPDATE": "SELECT",
	}
	for stmt, want := range tests {
		require.Equal(t, want, spanName(t.Context(), stmt), stmt)
	}
}
