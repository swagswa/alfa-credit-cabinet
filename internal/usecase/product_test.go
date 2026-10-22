package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

const rub = 100

func products(t *testing.T) usecase.ProductUseCase {
	t.Helper()
	c, err := rules.Products("../../rules/products.yaml")
	require.NoError(t, err)
	return usecase.NewProduct(c)
}

// строки таблицы подбора из модели процесса, раздел 3
func TestRecommendPicks(t *testing.T) {
	uc := products(t)
	wc, od, eq := domain.WorkingCapital, domain.Overdraft, domain.Equipment

	tests := []struct {
		name string
		q    usecase.RecommendQuery
		want []domain.ProductCode
	}{
		{"разрыв на 2 мес", q(domain.GoalCashGap, 1_000_000, 2, 0), []domain.ProductCode{od, wc}},
		{"разрыв на 1 мес", q(domain.GoalCashGap, 500_000, 1, 0), []domain.ProductCode{od, wc}},
		{"разрыв дольше 2 мес", q(domain.GoalCashGap, 1_000_000, 3, 0), []domain.ProductCode{wc, od}},
		{"закупка до половины оборота", q(domain.GoalPurchase, 1_000_000, 12, 2_500_000), []domain.ProductCode{wc, od}},
		{"закупка ровно половина оборота", q(domain.GoalPurchase, 1_000_000, 12, 2_000_000), []domain.ProductCode{wc}},
		{"закупка больше половины оборота", q(domain.GoalPurchase, 3_000_000, 12, 2_000_000), []domain.ProductCode{wc}},
		{"закупка, оборот не указан", q(domain.GoalPurchase, 3_000_000, 12, 0), []domain.ProductCode{wc, od}},
		{"ремонт", q(domain.GoalRenovation, 2_000_000, 24, 0), []domain.ProductCode{wc}},
		{"оборудование от 500 тыс", q(domain.GoalEquipment, 500_000, 24, 0), []domain.ProductCode{eq}},
		{"оборудование дешевле 500 тыс", q(domain.GoalEquipment, 400_000, 12, 0), []domain.ProductCode{eq, wc}},
		{"другое", q(domain.GoalOther, 1_000_000, 12, 0), []domain.ProductCode{wc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offers, err := uc.Recommend(tt.q)
			require.NoError(t, err)
			var got []domain.ProductCode
			for i, o := range offers {
				got = append(got, o.Product)
				require.Equal(t, i == 0, o.Primary)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

// суммы и сроки приводятся к диапазону продукта, платёж по базовой ставке
func TestRecommendOffers(t *testing.T) {
	uc := products(t)

	tests := []struct {
		name string
		q    usecase.RecommendQuery
		want []usecase.Offer
	}{
		{
			name: "овердрафт: срок договора 12, платёж это проценты за месяц",
			q:    q(domain.GoalCashGap, 1_000_000, 2, 0),
			want: []usecase.Offer{
				{Product: domain.Overdraft, Primary: true, AmountKop: 100_000_000, TermMonths: 12, RateBP: 2100,
					MonthlyPaymentKop: 1_750_000},
				{Product: domain.WorkingCapital, AmountKop: 100_000_000, TermMonths: 3, RateBP: 1900,
					MonthlyPaymentKop: 34_394_416},
			},
		},
		{
			name: "оборудование: сумма поднята до минимума продукта",
			q:    q(domain.GoalEquipment, 400_000, 12, 0),
			want: []usecase.Offer{
				{Product: domain.Equipment, Primary: true, AmountKop: 50_000_000, TermMonths: 12, RateBP: 1700,
					MonthlyPaymentKop: 4_560_238},
				{Product: domain.WorkingCapital, AmountKop: 40_000_000, TermMonths: 12, RateBP: 1900,
					MonthlyPaymentKop: 3_686_263},
			},
		},
		{
			name: "оборотный на 2,5 млн и 24 мес",
			q:    q(domain.GoalRenovation, 2_500_000, 24, 0),
			want: []usecase.Offer{
				{Product: domain.WorkingCapital, Primary: true, AmountKop: 250_000_000, TermMonths: 24, RateBP: 1900,
					MonthlyPaymentKop: 12_602_154},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offers, err := uc.Recommend(tt.q)
			require.NoError(t, err)
			require.Equal(t, tt.want, offers)
		})
	}
}

func TestRecommendInvalid(t *testing.T) {
	uc := products(t)
	for name, tt := range map[string]usecase.RecommendQuery{
		"пустая цель":      q("", 1_000_000, 12, 0),
		"неизвестная цель": q("leasing", 1_000_000, 12, 0),
		"нулевая сумма":    q(domain.GoalOther, 0, 12, 0),
		"нулевой срок":     q(domain.GoalOther, 1_000_000, 0, 0),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := uc.Recommend(tt)
			require.ErrorIs(t, err, domain.ErrInvalid)
		})
	}
}

func q(g domain.Goal, amountRub int64, term int, revenueRub int64) usecase.RecommendQuery {
	return usecase.RecommendQuery{
		Goal:              g,
		AmountKop:         amountRub * rub,
		TermMonths:        term,
		MonthlyRevenueKop: revenueRub * rub,
	}
}
