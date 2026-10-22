package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestValidINN(t *testing.T) {
	tests := []struct {
		inn  string
		want bool
	}{
		{"0000100012", true},
		{"0000500010", true},
		{"7701234560", true},
		{"000012345627", true},
		{"770123456703", true},
		{"0000100013", false},   // последняя цифра не та
		{"770123456704", false}, // сходится только 11-я
		{"770123456713", false}, // сходится только 12-я
		{"000010001", false},
		{"00001000120", false},
		{"00001000a2", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.inn, func(t *testing.T) {
			require.Equal(t, tt.want, domain.ValidINN(tt.inn))
		})
	}
}
