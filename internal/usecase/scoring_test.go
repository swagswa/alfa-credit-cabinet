package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// points раскладывает итог по факторам F1..F9
func points(s domain.Score) []int {
	out := make([]int, 0, len(s.Factors))
	for _, f := range s.Factors {
		out = append(out, f.Points)
	}
	return out
}

func TestScoreDemo(t *testing.T) {
	d := decisions(t)
	for _, c := range demoCases(t) {
		if c.Score == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			f := c.facts()
			s := d.Score(&f)
			require.Equal(t, c.Score.Factors, points(s))
			require.Equal(t, c.Score.Points, s.Points)
			require.Equal(t, c.Score.Grade, s.Grade)
			require.Equal(t, "2026-10-21.1", s.Version)
		})
	}
}

func TestScoreBins(t *testing.T) {
	d := decisions(t)
	tests := []struct {
		name   string
		facts  domain.Facts
		factor int // индекс F1..F9 с нуля
		want   int
	}{
		{"возраст ровно 12 мес", domain.Facts{BusinessAgeMonths: 12}, 0, 6},
		{"возраст ровно 60 мес", domain.Facts{BusinessAgeMonths: 60}, 0, 16},
		{"торговля 47", domain.Facts{OKVEDGroup: "47"}, 8, 6},
		{"стройка 41", domain.Facts{OKVEDGroup: "41"}, 8, 0},
		{"сельское хозяйство 01 не в списках", domain.Facts{OKVEDGroup: "01"}, 8, 3},
		{"платежи есть, выручки нет", domain.Facts{BKIPaymentsKop: 100}, 3, 0},
		{"ни платежей, ни выручки", domain.Facts{}, 3, 14},
		{"нагрузка ровно 0,1", domain.Facts{BKIPaymentsKop: 10, MonthlyRevenueKop: 100}, 3, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := d.Score(&tt.facts)
			require.Len(t, s.Factors, 9)
			require.Equal(t, tt.want, s.Factors[tt.factor].Points)
		})
	}
}
