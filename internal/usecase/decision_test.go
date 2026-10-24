package usecase_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// весь путь ядра по демо-компаниям: стоп-факторы, скоринг, политика
func TestDecision(t *testing.T) {
	uc := decisions(t)
	texts := map[string]string{
		"R_AMOUNT_DSR":    "Сумма снижена: платежи по всем кредитам не должны превышать 25% среднемесячной выручки",
		"R_TERM_EXTENDED": "Срок увеличен до 48 мес, чтобы ежемесячный платёж был посильным",
		"R_RATE_PREMIUM":  "Ставка 20,5%: базовая ставка продукта плюс надбавка за уровень риска",
	}
	for _, c := range demoCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			f := c.facts()
			d := uc.Check(&f)
			if d.Outcome == "" {
				s := uc.Score(&f)
				d = uc.Policy(c.application(), &f, &d, &s)
			}
			want := c.Decision
			require.Equal(t, want.Outcome, d.Outcome)
			requireTerms(t, want.Terms.terms(), d.Terms)
			requireTerms(t, want.Recommendation.terms(), d.Recommendation)

			var codes []string
			for _, code := range d.Codes() {
				if strings.HasPrefix(code, "R_") || strings.HasPrefix(code, "SF_") {
					codes = append(codes, code)
				}
			}
			require.ElementsMatch(t, want.Codes, codes)
			for _, sc := range want.SC {
				require.Contains(t, d.Codes(), sc)
			}
			if c.Name != "логистика" {
				return
			}
			for _, r := range d.Reasons {
				if text, ok := texts[r.Code]; ok {
					require.Equal(t, text, r.Text)
				}
			}
		})
	}
}

// одна кофейня класса A на разных продуктах (как в scoring_cases.yaml) и крайние случаи политики
func TestPolicy(t *testing.T) {
	uc := decisions(t)
	coffee := demoCases(t)[0]
	tests := []struct {
		name    string
		grade   string
		age     int
		noTurn  bool
		app     domain.Draft
		outcome domain.Status
		terms   domain.Terms // у manual_review это рекомендация, платёж не сверяем
		codes   []string
	}{
		{
			name:    "оборотный до 4,8 млн",
			grade:   "A",
			age:     40,
			app:     domain.Draft{Product: domain.WorkingCapital, AmountKop: 10_000_000_00, TermMonths: 36},
			outcome: domain.StatusManualReview,
			terms: domain.Terms{
				Product:    domain.WorkingCapital,
				AmountKop:  4_800_000_00,
				TermMonths: 36,
				RateBP:     1900,
				Guarantor:  true,
			},
			codes: []string{"R_MANUAL_AMOUNT", "R_AMOUNT_MULT", "R_GUARANTOR"},
		},
		{
			name:    "оборудование до 7,2 млн",
			grade:   "A",
			age:     40,
			app:     domain.Draft{Product: domain.Equipment, AmountKop: 15_000_000_00, TermMonths: 60},
			outcome: domain.StatusManualReview,
			terms: domain.Terms{
				Product:    domain.Equipment,
				AmountKop:  7_200_000_00,
				TermMonths: 60,
				RateBP:     1700,
				Pledge:     true,
			},
			codes: []string{"R_MANUAL_AMOUNT", "R_AMOUNT_MULT", "R_PLEDGE"},
		},
		{
			name:  "молодой бизнес, овердрафт до 650 тыс",
			grade: "A", age: 8,
			app:     domain.Draft{Product: domain.WorkingCapital, AmountKop: 1_000_000_00, TermMonths: 12},
			outcome: domain.StatusCounterOffer,
			terms:   domain.Terms{Product: domain.Overdraft, AmountKop: 650_000_00, TermMonths: 12, RateBP: 2100},
			codes:   []string{"R_SWITCH_AGE"},
		},
		{
			name:  "молодой бизнес без оборотов",
			grade: "A", age: 8, noTurn: true,
			app:     domain.Draft{Product: domain.WorkingCapital, AmountKop: 1_000_000_00, TermMonths: 12},
			outcome: domain.StatusRejected,
			codes:   []string{"R_AGE_PRODUCT"},
		},
		{
			name:    "овердрафт больше оборота, оборотный даёт больше",
			grade:   "A",
			age:     40,
			app:     domain.Draft{Product: domain.Overdraft, AmountKop: 1_000_000_00, TermMonths: 12},
			outcome: domain.StatusCounterOffer,
			terms: domain.Terms{
				Product:    domain.WorkingCapital,
				AmountKop:  1_000_000_00,
				TermMonths: 12,
				RateBP:     1900,
			},
			codes: []string{"R_SWITCH_TO_LOAN"},
		},
		{
			name:    "класс D",
			grade:   "D",
			age:     40,
			app:     domain.Draft{Product: domain.WorkingCapital, AmountKop: 500_000_00, TermMonths: 12},
			outcome: domain.StatusManualReview,
			terms: domain.Terms{
				Product:    domain.WorkingCapital,
				AmountKop:  500_000_00,
				TermMonths: 12,
				RateBP:     2500,
				Guarantor:  true,
			},
			codes: []string{"R_MANUAL_RISK", "R_GUARANTOR", "R_RATE_PREMIUM"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := coffee.facts()
			f.BusinessAgeMonths = tt.age
			if tt.noTurn {
				f.MonthlyTurnoverKop = 0
			}
			s := domain.Score{Grade: tt.grade}
			d := uc.Policy(&domain.Application{Draft: tt.app}, &f, &domain.Decision{}, &s)
			require.Equal(t, tt.outcome, d.Outcome)
			require.ElementsMatch(t, tt.codes, d.Codes())

			got := d.Terms
			if tt.outcome == domain.StatusManualReview {
				got = d.Recommendation
			}
			if tt.outcome == domain.StatusRejected {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			g := *got
			g.PaymentKop = 0
			require.Equal(t, tt.terms, g)
		})
	}
}

// платёж с точностью до рубля, остальное точно
func requireTerms(t *testing.T, want, got *domain.Terms) {
	t.Helper()
	if want == nil {
		require.Nil(t, got)
		return
	}
	require.NotNil(t, got)
	require.InDelta(t, want.PaymentKop, got.PaymentKop, 100)
	w, g := *want, *got
	w.PaymentKop, g.PaymentKop = 0, 0
	require.Equal(t, w, g)
}
