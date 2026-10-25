package mocks_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// TestOutcomes гоняет ядро решения по ответам моков для компаний из генератора, шаги те же,
// что в Decide. Заявка типичная: оборотный кредит на полторы месячные выручки на год,
// выручка в анкете честная, документы, которые просит кабинет, приложены.
// Доли исходов держим около 60/20/10/10, а балл должен отделять дефолты из разметки генератора.
func TestOutcomes(t *testing.T) {
	const n = 10000
	c, err := rules.Products("../../rules/products.yaml")
	require.NoError(t, err)
	s, err := rules.Scoring("../../rules/scoring.yaml")
	require.NoError(t, err)
	p, err := rules.Policy("../../rules/policy.yaml")
	require.NoError(t, err)
	dec := usecase.NewDecision(c, s, p)
	e := mocks.New()
	now := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)

	group := map[domain.Status]string{
		domain.StatusApproved:     "approved",
		domain.StatusCounterOffer: "counter",
		domain.StatusManualReview: "manual",
		domain.StatusNeedInfo:     "manual",
		domain.StatusRejected:     "rejected",
	}
	got := map[string]int{}
	var good, bad [101]int // число компаний по баллу
	for i := range n {
		// каждая пятая ИП
		inn := domain.CompleteINN(fmt.Sprintf("77%07d", i))
		if i%5 == 0 {
			inn = domain.CompleteINN(fmt.Sprintf("77%08d", i))
		}
		co, err := synth.New(inn)
		require.NoError(t, err)
		app := typical(co)
		for _, src := range domain.Sources {
			app.Checks = append(app.Checks, check(t, e, src, inn))
		}

		f, err := dec.Facts(&app, app.Checks, now)
		require.NoError(t, err, inn)
		d := dec.Check(&f)
		sc := dec.Score(&f)
		if d.Outcome == "" {
			d = dec.Policy(&app, &f, &d, &sc, time.Now())
		}
		got[group[d.Outcome]]++
		if co.Defaulted() {
			bad[sc.Points]++
		} else {
			good[sc.Points]++
		}
	}

	a := auc(good, bad)
	t.Logf("одобрено %d, встречное %d, ручное и запрос %d, отказ %d из %d; Gini %.3f, AUC %.3f",
		got["approved"], got["counter"], got["manual"], got["rejected"], n, 2*a-1, a)
	for k, want := range map[string]float64{"approved": 0.6, "counter": 0.2, "manual": 0.1, "rejected": 0.1} {
		require.InDelta(t, want, float64(got[k])/n, 0.05, k)
	}
	require.Greater(t, 2*a-1, 0.5)
}

func typical(c synth.Company) domain.Application {
	monthly := c.Income() / 12
	amount := min(max(monthly*3/2/1_000_000*1_000_000, 300_000*domain.KopInRub), 10_000_000*domain.KopInRub)
	months := make([]domain.MonthAmount, 0, len(c.Months))
	for _, m := range c.Months {
		months = append(months, domain.MonthAmount{Month: m.Start.Format("2006-01"), Amount: m.In})
	}
	return domain.Application{
		Product: domain.WorkingCapital, AmountKop: amount, TermMonths: 12, MonthlyRevenueKop: monthly,
		Documents: []domain.Document{
			{Kind: "passport"},
			{Kind: domain.DocFinancials},
			{Kind: domain.DocStatement, Status: domain.DocConfirmed, Confirmed: &domain.DocValues{
				Statement: &domain.Statement{TurnoverIn: c.Income(), MonthlyIn: months},
			}},
		},
	}
}

// check спрашивает мок так же, как клиент воркера, только без сети
func check(t *testing.T, e http.Handler, src domain.Source, inn string) domain.CheckResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/"+string(src)+"/"+inn, http.NoBody)
	if src == domain.SourceBKI {
		req = httptest.NewRequest(http.MethodPost, "/bki/report", strings.NewReader(`{"inn":"`+inn+`",`+consent+`}`))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	switch rec.Code {
	case http.StatusOK:
		return domain.CheckResult{Source: src, Status: domain.CheckOK, Payload: rec.Body.Bytes()}
	case http.StatusNotFound:
		return domain.CheckResult{Source: src, Status: domain.CheckNotFound}
	}
	t.Fatalf("%s %s: %d", src, inn, rec.Code)
	return domain.CheckResult{}
}

// auc по гистограмме баллов: доля пар (без дефолта, с дефолтом), где у первого балл выше, ничья за половину
func auc(good, bad [101]int) float64 {
	var pairs, below, ng, nb float64
	for p := range good {
		g, b := float64(good[p]), float64(bad[p])
		pairs += g * (below + b/2)
		below += b
		ng, nb = ng+g, nb+b
	}
	return pairs / (ng * nb)
}
