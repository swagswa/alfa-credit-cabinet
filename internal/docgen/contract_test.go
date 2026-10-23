package docgen_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// условия кофейни и логистики из демо, платёж посчитан руками по формуле аннуитета
var (
	coffee = domain.Terms{
		Product:    "working_capital",
		AmountKop:  250_000_000,
		TermMonths: 24,
		RateBP:     1900,
		PaymentKop: 12_602_154,
	}
	logistics = domain.Terms{
		Product:    "equipment",
		AmountKop:  390_000_000,
		TermMonths: 48,
		RateBP:     2050,
		PaymentKop: 11_971_984,
	}
)

func TestSchedule(t *testing.T) {
	for _, terms := range []domain.Terms{coffee, logistics} {
		rows := docgen.Schedule(terms)
		require.Len(t, rows, terms.TermMonths)
		var principal int64
		for i, r := range rows {
			require.Equal(t, i+1, r.Month)
			require.Equal(t, r.PaymentKop, r.InterestKop+r.PrincipalKop)
			if i < len(rows)-1 {
				require.Equal(t, terms.PaymentKop, r.PaymentKop)
			}
			principal += r.PrincipalKop
		}
		require.Equal(t, terms.AmountKop, principal)
		require.Zero(t, rows[len(rows)-1].BalanceKop)
		// последний платёж только добивает копейки округления
		require.InDelta(t, terms.PaymentKop, rows[len(rows)-1].PaymentKop, float64(terms.TermMonths))
	}
}

func TestContractPDF(t *testing.T) {
	c := docgen.Contract{
		Number: 1042, Date: time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC),
		Company: "ООО «Тёплое зерно»", INN: "0000100012", Terms: coffee, Repayment: domain.Annuity,
	}
	b, err := docgen.ContractPDF(c)
	require.NoError(t, err)
	txt := pdfText(t, b)
	for _, s := range []string{"Кредитный договор", "1042", "24.10.2026", "ООО «Тёплое зерно»", "0000100012",
		"2 500 000,00", "19,00", "126 021,54", "График платежей", "Alfa-Hack"} {
		require.Contains(t, txt, s)
	}
	again, err := docgen.ContractPDF(c)
	require.NoError(t, err)
	require.Equal(t, b, again, "повтор шага должен дать тот же файл")

	c.Terms = domain.Terms{Product: "overdraft", AmountKop: 150_000_000, TermMonths: 12, RateBP: 2300}
	c.Repayment = domain.AutoDebit
	b, err = docgen.ContractPDF(c)
	require.NoError(t, err)
	txt = pdfText(t, b)
	require.Contains(t, txt, "60 дней")
	require.False(t, strings.Contains(txt, "График платежей"), "у овердрафта графика нет")
}
