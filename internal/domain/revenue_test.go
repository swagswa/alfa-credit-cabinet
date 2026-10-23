package domain_test

import (
	"math"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

// порог берём из настоящей политики, чтобы граница в тесте не разъехалась с ней
func docMismatchLimit(t *testing.T) float64 {
	raw, err := os.ReadFile("../../rules/policy.yaml")
	require.NoError(t, err)
	var p struct {
		StopFactors []struct {
			Code string
			Gt   float64
		} `yaml:"stop_factors"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &p))
	for _, sf := range p.StopFactors {
		if sf.Code == domain.CodeDocMismatch {
			return sf.Gt
		}
	}
	t.Fatal("в policy.yaml нет " + domain.CodeDocMismatch)
	return 0
}

// выписка из тех же цифр, что отдаёт мок: последние n месяцев, приход умножен на k/2
func stmt(t *testing.T, inn string, n int, k int64) domain.Document {
	c, err := synth.New(inn)
	require.NoError(t, err)
	s := &domain.Statement{INN: inn}
	for _, m := range c.Months[len(c.Months)-n:] {
		in := m.In * k / 2
		s.TurnoverIn += in
		s.MonthlyIn = append(s.MonthlyIn, domain.MonthAmount{Month: m.Start.Format("2006-01"), Amount: in})
	}
	return confirmed(domain.DocStatement, domain.DocValues{Statement: s})
}

func confirmed(k domain.DocKind, v domain.DocValues) domain.Document {
	return domain.Document{ID: uuid.New(), Kind: k, Status: domain.DocConfirmed, Confirmed: &v}
}

func fns(t *testing.T, inn string) *domain.FNSIncome {
	c, err := synth.New(inn)
	require.NoError(t, err)
	return &domain.FNSIncome{Year: c.Year, Kop: c.Income()}
}

func TestCheckRevenue(t *testing.T) {
	limit := docMismatchLimit(t)
	const it = "0000300011"
	honest, trap := stmt(t, it, 12, 2), stmt(t, it, 12, 3)
	recognized := honest
	recognized.Status, recognized.Extracted, recognized.Confirmed = domain.DocRecognized, honest.Confirmed, nil

	itFNS := fns(t, it)
	report := func(year int, rev int64) domain.Document {
		return confirmed(
			domain.DocFinancials,
			domain.DocValues{Financials: &domain.Financials{Year: year, Revenue: rev}},
		)
	}
	// миллион в год у ФНС и отчётность ровно на пороге или на копейку выше
	edge := func(extra int64) domain.Document {
		return report(2025, 1_000_000_00+int64(math.Round(limit*1_000_000_00))+extra)
	}
	million := &domain.FNSIncome{Year: 2025, Kop: 1_000_000_00}

	tests := []struct {
		name string
		docs []domain.Document
		fns  *domain.FNSIncome
		want domain.RevenueResult
	}{
		{"кофейня", []domain.Document{stmt(t, "0000100012", 12, 2)}, fns(t, "0000100012"), domain.RevenueOK},
		{"логистика, ип", []domain.Document{stmt(t, "000020001010", 12, 2)}, fns(t, "000020001010"), domain.RevenueOK},
		{"it", []domain.Document{honest}, itFNS, domain.RevenueOK},
		{"стройка", []domain.Document{stmt(t, "0000400016", 12, 2)}, fns(t, "0000400016"), domain.RevenueOK},
		{"it, отчётность", []domain.Document{report(2025, itFNS.Kop)}, itFNS, domain.RevenueOK},
		{"ловушка it", []domain.Document{trap}, itFNS, domain.RevenueMismatch},
		{"ловушка рядом с честной", []domain.Document{honest, trap}, itFNS, domain.RevenueMismatch},
		// у it тренд 1,4: второе полугодие в годовом пересчёте выше ФНС на 17%, ещё в пороге
		{"полгода it", []domain.Document{stmt(t, it, 6, 2)}, itFNS, domain.RevenueOK},
		{"полгода ловушки", []domain.Document{stmt(t, it, 6, 3)}, itFNS, domain.RevenueMismatch},
		{"ровно порог", []domain.Document{edge(0)}, million, domain.RevenueOK},
		{"порог плюс копейка", []domain.Document{edge(1)}, million, domain.RevenueMismatch},
		{"нет документа", nil, itFNS, domain.RevenueNoData},
		{"распознан, не подтверждён", []domain.Document{recognized}, itFNS, domain.RevenueNoData},
		{"отчётность за другой год", []domain.Document{report(2024, itFNS.Kop*3)}, itFNS, domain.RevenueNoData},
		{"нет данных фнс", []domain.Document{trap}, nil, domain.RevenueNoData},
		{"фнс без выручки", []domain.Document{trap}, &domain.FNSIncome{Year: 2025}, domain.RevenueNoData},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.CheckRevenue(tt.docs, tt.fns, limit)
			require.Equal(t, tt.want, got.Result, "расхождение %.4f", got.Mismatch)
		})
	}
}

// карточке банка нужны обе цифры и что сравнивали
func TestCheckRevenueTrap(t *testing.T) {
	const it = "0000300011"
	f := fns(t, it)
	trap := stmt(t, it, 6, 3)
	got := domain.CheckRevenue([]domain.Document{stmt(t, it, 12, 2), trap}, f, docMismatchLimit(t))

	require.Equal(t, trap.ID, got.DocID)
	require.Equal(t, domain.DocStatement, got.Kind)
	require.Equal(t, 6, got.Months)
	require.Equal(t, f.Kop, got.FNSKop)
	require.Equal(t, trap.Confirmed.Statement.TurnoverIn*2, got.DocKop)
	require.InDelta(t, 0.75, got.Mismatch, 0.01)
}
