package activity_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/source"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

// cardApps отдаёт карточку заявки, как её прочитал бы Get
type cardApps struct {
	usecase.ApplicationUseCase
	app domain.Application
}

func (f cardApps) Get(context.Context, domain.Actor, uuid.UUID) (domain.Application, error) {
	return f.app, nil
}

// Решение на настоящих правилах из rules/ и ответах моков, как на стенде.
func TestDecide(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "rules")
	catalog, err := rules.Products(filepath.Join(dir, "products.yaml"))
	require.NoError(t, err)
	scoring, err := rules.Scoring(filepath.Join(dir, "scoring.yaml"))
	require.NoError(t, err)
	policy, err := rules.Policy(filepath.Join(dir, "policy.yaml"))
	require.NoError(t, err)
	decisions := usecase.NewDecision(catalog, scoring, policy)

	srv := httptest.NewServer(mocks.New())
	t.Cleanup(srv.Close)
	src := source.New(source.URLs{EGRUL: srv.URL, FNS: srv.URL, BKI: srv.URL, Bank: srv.URL}, 5*time.Second)

	const mln = 1_000_000 * domain.KopInRub
	file := func(kind domain.DocKind) domain.Document {
		return domain.Document{Kind: kind, Status: domain.DocUploaded}
	}
	tests := []struct {
		name    string
		inn     string
		draft   domain.Draft
		docs    []domain.Document
		outcome domain.Status
		codes   []string // без SC_*, их проверяют тесты скоринга
		amount  int64    // сумма в условиях или рекомендации
	}{
		{
			"кофейня",
			"0000100012",
			domain.Draft{
				Product:           domain.WorkingCapital,
				AmountKop:         2.5 * mln,
				TermMonths:        24,
				MonthlyRevenueKop: 1.2 * mln,
			},
			[]domain.Document{file("passport")},
			domain.StatusApproved,
			[]string{"R_APPROVED"},
			2.5 * mln,
		},
		{
			"логистика", "000020001010",
			domain.Draft{
				Product: domain.Equipment, AmountKop: 6 * mln, TermMonths: 36, MonthlyRevenueKop: 2 * mln,
				EquipmentCostKop: 6.7 * mln,
			},
			[]domain.Document{file("passport"), file("supply_contract"), file(domain.DocFinancials)},
			domain.StatusCounterOffer, []string{"R_TERM_EXTENDED", "R_AMOUNT_DSR", "R_PLEDGE", "R_RATE_PREMIUM"},
			3.9 * mln,
		},
		{
			"IT без выписки и отчётности",
			"0000300011",
			domain.Draft{
				Product:           domain.WorkingCapital,
				AmountKop:         8 * mln,
				TermMonths:        12,
				MonthlyRevenueKop: 3.5 * mln,
			},
			[]domain.Document{file("passport")},
			domain.StatusNeedInfo,
			[]string{"SF_DOCS_MISSING"},
			0,
		},
		{
			"IT с документами",
			"0000300011",
			domain.Draft{
				Product:           domain.WorkingCapital,
				AmountKop:         8 * mln,
				TermMonths:        12,
				MonthlyRevenueKop: 3.5 * mln,
			},
			append(confirmed(t, "0000300011"), file("passport")),
			// в scoring_cases.yaml лимит 2M = 7 млн, но приход по выписке генератора чуть меньше 3,5 млн в месяц
			domain.StatusManualReview,
			[]string{"R_MANUAL_AMOUNT", "R_AMOUNT_MULT", "R_GUARANTOR", "R_RATE_PREMIUM"},
			6.99 * mln,
		},
		{
			"стройка",
			"0000400016",
			domain.Draft{
				Product:           domain.WorkingCapital,
				AmountKop:         5 * mln,
				TermMonths:        12,
				MonthlyRevenueKop: 4 * mln,
			},
			[]domain.Document{file("passport"), file(domain.DocFinancials)},
			domain.StatusRejected,
			[]string{"R_GRADE_E", "R_FIX_TAX"},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := domain.Application{Draft: tt.draft, Documents: tt.docs}
			for _, s := range domain.Sources {
				body, err := src.Fetch(t.Context(), s, tt.inn, domain.BKIConsent{ID: uuid.New(), GivenAt: time.Now()})
				c := domain.CheckResult{Source: s, Status: domain.CheckOK, Payload: body}
				// у IT нет счёта в банке
				if errors.Is(err, domain.ErrNotFound) {
					c, err = domain.CheckResult{Source: s, Status: domain.CheckNotFound}, nil
				}
				require.NoError(t, err)
				app.Checks = append(app.Checks, c)
			}
			a := activity.NewApplicationActivities(cardApps{app: app}, decisions, nil, domain.Deadlines{})

			d, err := a.Decide(t.Context(), uuid.New())
			require.NoError(t, err)
			require.Equal(t, tt.outcome, d.Outcome)
			var codes []string
			for _, c := range d.Codes() {
				if !strings.HasPrefix(c, "SC_") {
					codes = append(codes, c)
				}
			}
			require.ElementsMatch(t, tt.codes, codes)

			terms := d.Terms
			if terms == nil {
				terms = d.Recommendation
			}
			if tt.amount == 0 {
				require.Nil(t, terms)
				return
			}
			require.Equal(t, tt.amount, terms.AmountKop)
		})
	}
}

// confirmed это выписка и отчётность с цифрами генератора, как их подтвердил бы клиент
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
