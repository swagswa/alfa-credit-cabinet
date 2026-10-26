package usecase_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

func guidance(t *testing.T, sources usecase.Sources) usecase.GuidanceUseCase {
	t.Helper()
	c, err := rules.Products("../../rules/products.yaml")
	require.NoError(t, err)
	s, err := rules.Scoring("../../rules/scoring.yaml")
	require.NoError(t, err)
	r, err := rules.Guidance("../../rules/guidance.yaml")
	require.NoError(t, err)
	users := &fakeUsers{companies: []domain.ProfileCompany{{INN: "0000100012"}}}
	return usecase.NewGuidance(users, sources, c, s, r)
}

func TestImprovements(t *testing.T) {
	u, engine := guidance(t, nil), decisions(t)
	for _, c := range demoCases(t) {
		if c.Score == nil {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			facts := c.facts()
			s := engine.Score(&facts)
			original := s
			original.Factors = append([]domain.FactorScore(nil), s.Factors...)
			app := &domain.Application{Status: domain.StatusRejected, Decision: &domain.Decision{Score: &s}}
			p := u.Improvements(app)
			require.True(t, p.Available)
			if c.Name == "стройка" {
				require.Len(t, p.Items, 5)
				require.Equal(t, 8, p.Items[0].Delta)
				require.Equal(t, 33, p.Items[0].Score)
				require.Equal(t, "D", p.Items[1].Grade)
			}
			for _, step := range p.Items {
				f := facts
				switch step.Input {
				case "tax_arrears_rub":
					f.TaxArrearsKop = int64(step.Target * 100)
				case "current_dsr":
					f.BKIPaymentsKop = int64(step.Target * float64(f.MonthlyRevenueKop))
				case "revenue_trend":
					f.RevenueTrend = step.Target
				case "turnover_cv":
					f.TurnoverCV = step.Target
				case "inquiries_90d":
					f.Inquiries90d = int(step.Target)
				default:
					t.Fatalf("неизвестный фактор %s", step.Input)
				}
				recalculated := engine.Score(&f)
				require.Equal(t, recalculated.Points, step.Score)
				require.Equal(t, recalculated.Grade, step.Grade)
				require.Equal(t, recalculated.Points-s.Points, step.Delta)
				require.Positive(t, step.Delta)
			}
			require.Equal(t, original, s, "снимок решения не меняется")
			app.Status = domain.StatusApproved
			require.False(t, u.Improvements(app).Available)
			app.Status = domain.StatusRejected
			s.Version = "старая версия"
			require.False(t, u.Improvements(app).Available)
		})
	}
	require.False(t, u.Improvements(&domain.Application{Status: domain.StatusRejected}).Available)
}

type estimateSources struct {
	tax, bank string
	fail      domain.Source
	called    []domain.Source
}

func (s *estimateSources) Fetch(
	_ context.Context,
	source domain.Source,
	_ string,
	_ domain.BKIConsent,
) (json.RawMessage, error) {
	s.called = append(s.called, source)
	if source == s.fail {
		return nil, domain.ErrUnavailable
	}
	if source == domain.SourceFNS {
		return json.RawMessage(s.tax), nil
	}
	if source == domain.SourceInternal {
		return json.RawMessage(s.bank), nil
	}
	panic("БКИ до согласия вызываться не должен")
}

func TestPreliminary(t *testing.T) {
	for _, tc := range []struct {
		name, tax, bank    string
		fail               domain.Source
		want               int64
		empty, unavailable bool
	}{
		{name: "меньшее из ФНС и счёта", tax: `{"income_last_year":14400000}`, bank: `{"is_client":true,"turnover_12m":[{"credit":1500000}]}`, want: 120000000},
		{name: "счёт ограничивает", tax: `{"income_last_year":14400000}`, bank: `{"is_client":true,"turnover_12m":[{"credit":600000}]}`, want: 60000000},
		{name: "не клиент банка", tax: `{"income_last_year":14400000}`, bank: `{"is_client":false}`, want: 120000000},
		{name: "налоговый долг", tax: `{"income_last_year":14400000,"arrears":{"tax":1}}`, bank: `{"is_client":false}`, want: 120000000, empty: true},
		{name: "нет выручки", tax: `{}`, bank: `{"is_client":false}`, unavailable: true},
		{name: "ФНС недоступна", fail: domain.SourceFNS, unavailable: true},
		{name: "банк недоступен", tax: `{"income_last_year":14400000}`, fail: domain.SourceInternal, unavailable: true},
		{name: "битый ответ", tax: `no json`, bank: `{"is_client":false}`, unavailable: true},
		{name: "пустой ряд", tax: `{"income_last_year":14400000}`, bank: `{"is_client":true}`, unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := &estimateSources{tax: tc.tax, bank: tc.bank, fail: tc.fail}
			u := guidance(t, src)
			actor := domain.Actor{ID: uuid.New(), Role: domain.RoleClient}
			_, err := u.Preliminary(t.Context(), actor, "0000400016")
			require.ErrorIs(t, err, domain.ErrNotFound)
			require.Empty(t, src.called)
			p, err := u.Preliminary(t.Context(), actor, "0000100012")
			if tc.unavailable {
				require.ErrorIs(t, err, domain.ErrUnavailable)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, p.MonthlyRevenueKop)
			require.Equal(t, []domain.Source{domain.SourceFNS, domain.SourceInternal}, src.called)
			if tc.empty {
				require.Empty(t, p.Items)
			} else {
				require.NotEmpty(t, p.Items)
				require.Equal(t, tc.want*2, p.Items[0].AmountKop)
			}
		})
	}
}
