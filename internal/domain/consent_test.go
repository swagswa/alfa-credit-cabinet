package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestBKIConsentValid(t *testing.T) {
	given := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	id := uuid.New()
	tests := []struct {
		name string
		c    domain.BKIConsent
		now  time.Time
		want bool
	}{
		{"только что дали", domain.BKIConsent{ID: id, GivenAt: given}, given, true},
		{"за минуту до конца", domain.BKIConsent{ID: id, GivenAt: given}, end.Add(-time.Minute), true},
		{"ровно полгода", domain.BKIConsent{ID: id, GivenAt: given}, end, false},
		{"без id", domain.BKIConsent{GivenAt: given}, given, false},
		{"без даты", domain.BKIConsent{ID: id}, given, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.c.Valid(tt.now))
		})
	}
}
