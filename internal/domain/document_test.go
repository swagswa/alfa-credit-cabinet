package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

var today = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)

// квартал: 100 + 900 - 400 = 600, приход по месяцам 200 + 300 + 400
func statement() domain.Statement {
	return domain.Statement{
		INN:            "0000100012",
		PeriodFrom:     "2026-07-01",
		PeriodTo:       "2026-09-30",
		OpeningBalance: 100_00,
		TurnoverIn:     900_00,
		TurnoverOut:    400_00,
		ClosingBalance: 600_00,
		MonthlyIn: []domain.MonthAmount{
			{"2026-07", 200_00}, {"2026-08", 300_00}, {"2026-09", 400_00},
		},
	}
}

func TestStatementCheck(t *testing.T) {
	tests := []struct {
		name   string
		change func(s *domain.Statement)
		want   []string // поля с ошибками
	}{
		{"всё сходится", func(*domain.Statement) {}, nil},
		{"копейки потерялись", func(s *domain.Statement) { s.ClosingBalance += 99; s.MonthlyIn[0].Amount -= 50 }, nil},
		{"чужой инн", func(s *domain.Statement) { s.INN = "0000300011" }, []string{"inn"}},
		{
			"остаток не сходится",
			func(s *domain.Statement) { s.ClosingBalance = 700_00 },
			[]string{"closing_balance_kop"},
		},
		{"нет прихода", func(s *domain.Statement) {
			s.TurnoverIn, s.ClosingBalance = 0, -300_00
			s.MonthlyIn = []domain.MonthAmount{{"2026-07", 0}, {"2026-08", 0}, {"2026-09", 0}}
		}, []string{"turnover_in_kop"}},
		{"отрицательный расход", func(s *domain.Statement) { s.TurnoverOut, s.ClosingBalance = -1, 1000_01 },
			[]string{"turnover_out_kop"}},
		{"дата не дата", func(s *domain.Statement) { s.PeriodTo = "30.09.2026" }, []string{"period_from"}},
		{"конец раньше начала", func(s *domain.Statement) { s.PeriodTo = "2026-06-30" },
			[]string{"period_from", "monthly_in"}},
		{"в будущем", func(s *domain.Statement) {
			s.PeriodFrom, s.PeriodTo = "2026-09-01", "2026-11-30"
			s.MonthlyIn = []domain.MonthAmount{{"2026-09", 200_00}, {"2026-10", 300_00}, {"2026-11", 400_00}}
		}, []string{"period_to"}},
		{"длиннее 13 месяцев", func(s *domain.Statement) { s.PeriodFrom = "2025-01-01" },
			[]string{"period_from", "monthly_in"}},
		{"дыра в месяцах", func(s *domain.Statement) { s.MonthlyIn[1].Month = "2026-09" }, []string{"monthly_in"}},
		{"месяца не хватает", func(s *domain.Statement) {
			s.MonthlyIn = s.MonthlyIn[:2]
			s.MonthlyIn[1].Amount = 700_00
		}, []string{"monthly_in"}},
		{"сумма по месяцам мимо", func(s *domain.Statement) { s.MonthlyIn[2].Amount = 500_00 }, []string{"monthly_in"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := statement()
			tt.change(&s)
			require.Equal(t, tt.want, fields(s.Check("0000100012", today)))
		})
	}
}

func TestFinancialsCheck(t *testing.T) {
	ok := domain.Financials{
		INN: "0000100012", Year: 2025, Revenue: 1000_00, NetProfit: 100_00,
		LiabilitiesLong: 200_00, LiabilitiesShort: 300_00, BalanceTotal: 900_00,
	}
	tests := []struct {
		name   string
		change func(f *domain.Financials)
		want   []string
	}{
		{"всё сходится", func(*domain.Financials) {}, nil},
		{"позапрошлый год", func(f *domain.Financials) { f.Year = 2024 }, nil},
		{"убыток", func(f *domain.Financials) { f.NetProfit = -900_00 }, nil},
		{"чужой инн", func(f *domain.Financials) { f.INN = "0000300011" }, []string{"inn"}},
		{"текущий год", func(f *domain.Financials) { f.Year = 2026 }, []string{"year"}},
		{"старая", func(f *domain.Financials) { f.Year = 2023 }, []string{"year"}},
		{"убыток больше выручки", func(f *domain.Financials) { f.NetProfit = -1000_01 }, []string{"net_profit_kop"}},
		{"отрицательная выручка", func(f *domain.Financials) { f.Revenue, f.NetProfit = -1, 0 },
			[]string{"revenue_kop", "net_profit_kop"}},
		{"отрицательный долг", func(f *domain.Financials) { f.LiabilitiesLong = -1 },
			[]string{"liabilities_long_kop"}},
		{"баланс меньше долгов", func(f *domain.Financials) { f.BalanceTotal = 499_99 }, []string{"balance_total_kop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := ok
			tt.change(&f)
			require.Equal(t, tt.want, fields(f.Check("0000100012", today)))
		})
	}
}

func fields(errs []domain.FieldError) []string {
	var out []string
	for _, e := range errs {
		out = append(out, e.Field)
	}
	return out
}

func TestReconcile(t *testing.T) {
	// второе прочтение другим режимом OCR: часть цифр верна там, часть здесь
	tests := []struct {
		name   string
		first  func(s *domain.Statement)
		second func(s *domain.Statement)
		want   bool
		from   []string // поля, которые должны прийти из второго прочтения
	}{
		{
			"первое и так сходится",
			func(*domain.Statement) {},
			func(s *domain.Statement) { s.TurnoverIn = 1 },
			true,
			nil,
		},
		{"одна цифра прихода", func(s *domain.Statement) { s.TurnoverIn = 950_00 },
			func(s *domain.Statement) { s.ClosingBalance = 0 }, true, []string{"turnover_in_kop"}},
		{"по одной ошибке в каждом", func(s *domain.Statement) { s.TurnoverIn = 950_00 },
			func(s *domain.Statement) { s.OpeningBalance = 0 }, true, []string{"turnover_in_kop"}},
		{
			"месяц и остаток из второго",
			func(s *domain.Statement) { s.MonthlyIn[1].Amount = 800_00; s.ClosingBalance = 1 },
			func(s *domain.Statement) { s.TurnoverOut = 0 },
			true,
			[]string{"closing_balance_kop", "monthly_in"},
		},
		{"оба прочтения мимо в одном поле", func(s *domain.Statement) { s.TurnoverIn = 950_00 },
			func(s *domain.Statement) { s.TurnoverIn = 990_00 }, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := statement(), statement()
			tt.first(&a)
			tt.second(&b)
			got, ok := domain.DocValues{Statement: &a}.Reconcile(domain.DocValues{Statement: &b}, "0000100012", today)
			require.Equal(t, tt.want, ok)
			if !ok {
				require.Equal(t, domain.DocValues{Statement: &a}, got, "не сошлось: остаётся первое прочтение")
				return
			}
			want := statement()
			require.Equal(t, &want, got.Statement)
			require.Equal(t, tt.from, got.Edited(&domain.DocValues{Statement: &a}))
		})
	}
}
