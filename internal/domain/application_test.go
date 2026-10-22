package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestReady(t *testing.T) {
	catalog := domain.Catalog{
		AmountStepKop: 1000000,
		Products: []domain.Product{
			{Code: domain.WorkingCapital, AmountMinKop: 30000000, AmountMaxKop: 1000000000,
				TermMinMonths: 3, TermMaxMonths: 36},
			{Code: domain.Equipment, AmountMinKop: 50000000, AmountMaxKop: 1500000000,
				TermMinMonths: 12, TermMaxMonths: 60, MaxCostShare: 0.9},
		},
	}
	full := domain.Draft{
		Product: domain.WorkingCapital, AmountKop: 250000000, TermMonths: 24, BKIConsent: true, PDConsent: true,
	}

	tests := []struct {
		name   string
		edit   func(*domain.Draft)
		fields []string
	}{
		{"всё на месте", func(*domain.Draft) {}, nil},
		{"сумма меньше минимума", func(d *domain.Draft) { d.AmountKop = 20000000 }, []string{"amount_kop"}},
		{"сумма больше максимума", func(d *domain.Draft) { d.AmountKop = 2000000000 }, []string{"amount_kop"}},
		{"сумма не кратна шагу", func(d *domain.Draft) { d.AmountKop = 250500000 }, []string{"amount_kop"}},
		{"срок больше продукта", func(d *domain.Draft) { d.TermMonths = 48 }, []string{"term_months"}},
		{"без согласий", func(d *domain.Draft) { d.BKIConsent, d.PDConsent = false, false },
			[]string{"bki_consent", "pd_consent"}},
		{"оборудование без стоимости", func(d *domain.Draft) { d.Product = domain.Equipment },
			[]string{"equipment_cost_kop"}},
		{"кредит больше 90% стоимости", func(d *domain.Draft) {
			d.Product, d.EquipmentCostKop = domain.Equipment, 270000000
		}, []string{"amount_kop"}},
		{"ровно 90% стоимости", func(d *domain.Draft) {
			d.Product, d.AmountKop, d.EquipmentCostKop = domain.Equipment, 270000000, 300000000
		}, nil},
		{"продукта нет в каталоге", func(d *domain.Draft) { d.Product = domain.Overdraft }, []string{"product"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := full
			tt.edit(&d)
			var fields []string
			for _, e := range catalog.Ready(&d) {
				require.NotEmpty(t, e.Message)
				fields = append(fields, e.Field)
			}
			require.Equal(t, tt.fields, fields)
		})
	}
}
