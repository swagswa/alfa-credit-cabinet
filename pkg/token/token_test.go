package token

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIssuer(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	iss := New("secret", time.Hour)
	iss.now = func() time.Time { return now }

	raw, err := iss.Issue("s1")
	require.NoError(t, err)

	sid, err := iss.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "s1", sid)

	tests := []struct {
		name string
		iss  *Issuer
		at   time.Time
	}{
		{"чужой секрет", New("other", time.Hour), now},
		{"истёк", iss, now.Add(2 * time.Hour)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.iss.now = func() time.Time { return tt.at }
			_, err := tt.iss.Parse(raw)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}
