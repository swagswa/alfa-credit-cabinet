package usecase_test

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/source"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

var passport = domain.Document{Kind: "passport", Status: domain.DocUploaded}

// fetch опрашивает моки так же, как workflow, и складывает ответы как check_results
func fetch(t *testing.T, inn string) []domain.CheckResult {
	t.Helper()
	srv := httptest.NewServer(mocks.New())
	t.Cleanup(srv.Close)
	c := source.New(source.URLs{EGRUL: srv.URL, FNS: srv.URL, BKI: srv.URL, Bank: srv.URL}, 5*time.Second)

	var out []domain.CheckResult
	for _, src := range domain.Sources {
		body, err := c.Fetch(t.Context(), src, inn, domain.BKIConsent{ID: uuid.New(), GivenAt: time.Now()})
		switch {
		case err == nil:
			out = append(out, domain.CheckResult{Source: src, Status: domain.CheckOK, Payload: body})
		case errors.Is(err, domain.ErrNotFound):
			out = append(out, domain.CheckResult{Source: src, Status: domain.CheckNotFound})
		default:
			require.NoError(t, err)
		}
	}
	return out
}

// confirmed это выписка за 12 мес и отчётность за 2025 с цифрами генератора, как их подтвердил бы клиент
func confirmed(t *testing.T, inn string) []domain.Document {
	t.Helper()
	c, err := synth.New(inn)
	require.NoError(t, err)
	st := &domain.Statement{INN: inn}
	for _, m := range c.Months {
		st.MonthlyIn = append(st.MonthlyIn, domain.MonthAmount{Month: m.Start.Format("2006-01"), Amount: m.In})
		st.TurnoverIn += m.In
	}
	fin := &domain.Financials{INN: inn, Year: c.Year, Revenue: c.Income()}
	return []domain.Document{
		{Kind: domain.DocStatement, Status: domain.DocConfirmed, Confirmed: &domain.DocValues{Statement: st}},
		{Kind: domain.DocFinancials, Status: domain.DocConfirmed, Confirmed: &domain.DocValues{Financials: fin}},
	}
}

// Сквозная проверка: живые ответы моков -> факты -> балл, ожидания из testdata/scoring_cases.yaml.
func TestFactsDemo(t *testing.T) {
	d := decisions(t)
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	const mln = 1_000_000 * domain.KopInRub
	type want struct {
		age, history, dpd, inquiries int
		okved                        string
		missing                      []string
		factors                      []int
		points                       int
		grade                        string
	}
	tests := []struct {
		name  string
		inn   string
		draft domain.Draft
		docs  []domain.Document
		want  want
	}{
		{
			"кофейня", "0000100012",
			domain.Draft{Product: domain.WorkingCapital, AmountKop: 2.5 * mln, MonthlyRevenueKop: 1.2 * mln},
			[]domain.Document{passport},
			want{40, 2, 0, 1, "56", []string{}, []int{12, 10, 10, 14, 16, 8, 8, 6, 3}, 87, "A"},
		},
		{
			// 6 млн больше 3 млн, по правилам нужна отчётность, у ИП это декларация
			"логистика", "000020001010",
			domain.Draft{Product: domain.Equipment, AmountKop: 6 * mln, MonthlyRevenueKop: 2 * mln},
			[]domain.Document{passport, {Kind: "supply_contract", Status: domain.DocUploaded}},
			want{30, 1, 1, 4, "49", []string{"financials"}, []int{12, 10, 5, 10, 9, 4, 8, 3, 3}, 64, "C"},
		},
		{
			"IT до документов", "0000300011",
			domain.Draft{Product: domain.WorkingCapital, AmountKop: 8 * mln, MonthlyRevenueKop: 3.5 * mln},
			[]domain.Document{passport},
			want{20, 0, 0, 7, "62", []string{"statement", "financials"}, nil, 0, ""},
		},
		{
			"IT после документов", "0000300011",
			domain.Draft{Product: domain.WorkingCapital, AmountKop: 8 * mln, MonthlyRevenueKop: 3.5 * mln},
			append(confirmed(t, "0000300011"), passport),
			want{20, 0, 0, 7, "62", []string{}, []int{6, 14, 0, 14, 16, 0, 4, 0, 6}, 60, "C"},
		},
		{
			// в scoring_cases.yaml F8 = 6 и 25 баллов, но в фикстуре у нас погашен кредит без просрочек: уровень 3, 8 баллов
			"стройка", "0000400016",
			domain.Draft{Product: domain.WorkingCapital, AmountKop: 5 * mln, MonthlyRevenueKop: 4 * mln},
			append(confirmed(t, "0000400016"), passport),
			want{70, 3, 30, 9, "41", []string{}, []int{16, 0, 0, 0, 3, 0, 0, 8, 0}, 27, "E"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &domain.Application{Draft: tt.draft, Documents: tt.docs}
			f, err := d.Facts(app, fetch(t, tt.inn), now)
			require.NoError(t, err)
			w := tt.want
			require.True(t, f.EGRULFound)
			require.Equal(t, w.age, f.BusinessAgeMonths)
			require.Equal(t, w.okved, f.OKVEDGroup)
			require.Equal(t, w.history, f.BankHistoryLevel)
			require.Equal(t, w.dpd, f.WorstDPD24m)
			require.Equal(t, w.inquiries, f.Inquiries90d)
			require.Equal(t, w.missing, f.MissingDocuments)
			require.Zero(t, f.SourcesUnavailable)
			require.Less(t, f.DocRevenueMismatch, 0.01)
			if w.factors == nil {
				return
			}
			s := d.Score(&f)
			require.Equal(t, w.factors, points(s))
			require.Equal(t, w.points, s.Points)
			require.Equal(t, w.grade, s.Grade)
		})
	}
}

func TestFactsWithoutSources(t *testing.T) {
	d := decisions(t)
	app := &domain.Application{Role: "proxy", Product: domain.Overdraft, MonthlyRevenueKop: 500_000_00}
	checks := []domain.CheckResult{
		{Source: domain.SourceEGRUL, Status: domain.CheckUnavailable},
		{Source: domain.SourceFNS, Status: domain.CheckUnavailable},
		{Source: domain.SourceInternal, Status: domain.CheckNotFound},
	}
	f, err := d.Facts(app, checks, time.Now())
	require.NoError(t, err)
	require.False(t, f.EGRULFound)
	require.Equal(t, 2, f.SourcesUnavailable)
	// сверить не с чем, остаётся заявленная выручка
	require.Equal(t, int64(500_000_00), f.MonthlyRevenueKop)
	require.Equal(t, []string{"passport", "statement", "power_of_attorney"}, f.MissingDocuments)
}
