package usecase_test

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// demoCase строка testdata/scoring_cases.yaml
type demoCase struct {
	Name  string `yaml:"name"`
	INN   string `yaml:"inn"`
	Facts struct {
		IsIP               bool     `yaml:"is_ip"`
		BusinessAgeMonths  int      `yaml:"business_age_months"`
		OKVEDGroup         string   `yaml:"okved_group"`
		MonthlyRevenueRub  int64    `yaml:"monthly_revenue_rub"`
		MonthlyTurnoverRub int64    `yaml:"monthly_turnover_rub"`
		RevenueTrend       float64  `yaml:"revenue_trend"`
		TurnoverCV         float64  `yaml:"turnover_cv"`
		BKIPaymentsRub     int64    `yaml:"bki_payments_rub"`
		WorstDPD24m        int      `yaml:"worst_dpd_24m"`
		CurrentDPDMax      int      `yaml:"current_dpd_max"`
		Inquiries90d       int      `yaml:"inquiries_90d"`
		TaxArrearsRub      int64    `yaml:"tax_arrears_rub"`
		BankHistoryLevel   int      `yaml:"bank_history_level"`
		MissingDocuments   []string `yaml:"missing_documents"`
		DocRevenueMismatch float64  `yaml:"doc_revenue_mismatch"`
	} `yaml:"facts"`
	Score *struct {
		Factors []int  `yaml:"factors"`
		Points  int    `yaml:"points"`
		Grade   string `yaml:"grade"`
	} `yaml:"score"`
	Application struct {
		Product          domain.ProductCode `yaml:"product"`
		AmountRub        int64              `yaml:"amount_rub"`
		TermMonths       int                `yaml:"term_months"`
		EquipmentCostRub int64              `yaml:"equipment_cost_rub"`
	} `yaml:"application"`
	Decision struct {
		Outcome        domain.Status `yaml:"outcome"`
		Terms          *caseTerms    `yaml:"terms"`
		Recommendation *caseTerms    `yaml:"recommendation"`
		Codes          []string      `yaml:"codes"`
		SC             []string      `yaml:"sc"`
	} `yaml:"decision"`
}

type caseTerms struct {
	Product    domain.ProductCode `yaml:"product"`
	AmountRub  int64              `yaml:"amount_rub"`
	TermMonths int                `yaml:"term_months"`
	RatePct    float64            `yaml:"rate_pct"`
	PaymentRub int64              `yaml:"payment_rub"`
	Guarantor  bool               `yaml:"guarantor"`
	Pledge     bool               `yaml:"pledge"`
}

func demoCases(t *testing.T) []demoCase {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/scoring_cases.yaml")
	require.NoError(t, err)
	var cases []demoCase
	require.NoError(t, yaml.Unmarshal(raw, &cases))
	return cases
}

func (c *demoCase) facts() domain.Facts {
	f := c.Facts
	return domain.Facts{
		IsIP:               f.IsIP,
		EGRULFound:         true,
		EGRULStatus:        "active",
		BusinessAgeMonths:  f.BusinessAgeMonths,
		OKVEDGroup:         f.OKVEDGroup,
		MonthlyRevenueKop:  f.MonthlyRevenueRub * domain.KopInRub,
		MonthlyTurnoverKop: f.MonthlyTurnoverRub * domain.KopInRub,
		RevenueTrend:       f.RevenueTrend,
		TurnoverCV:         f.TurnoverCV,
		BKIPaymentsKop:     f.BKIPaymentsRub * domain.KopInRub,
		WorstDPD24m:        f.WorstDPD24m,
		CurrentDPDMax:      f.CurrentDPDMax,
		Inquiries90d:       f.Inquiries90d,
		TaxArrearsKop:      f.TaxArrearsRub * domain.KopInRub,
		BankHistoryLevel:   f.BankHistoryLevel,
		MissingDocuments:   f.MissingDocuments,
		DocRevenueMismatch: f.DocRevenueMismatch,
	}
}

func (c *demoCase) application() *domain.Application {
	a := c.Application
	return &domain.Application{
		Product:          a.Product,
		AmountKop:        a.AmountRub * domain.KopInRub,
		TermMonths:       a.TermMonths,
		EquipmentCostKop: a.EquipmentCostRub * domain.KopInRub,
	}
}

func (c *caseTerms) terms() *domain.Terms {
	if c == nil {
		return nil
	}
	return &domain.Terms{
		Product:    c.Product,
		AmountKop:  c.AmountRub * domain.KopInRub,
		TermMonths: c.TermMonths,
		RateBP:     int(math.Round(c.RatePct * 100)),
		PaymentKop: c.PaymentRub * domain.KopInRub,
		Guarantor:  c.Guarantor,
		Pledge:     c.Pledge,
	}
}

func decisions(t *testing.T) usecase.DecisionUseCase {
	t.Helper()
	c, err := rules.Products("../../rules/products.yaml")
	require.NoError(t, err)
	s, err := rules.Scoring("../../rules/scoring.yaml")
	require.NoError(t, err)
	p, err := rules.Policy("../../rules/policy.yaml")
	require.NoError(t, err)
	return usecase.NewDecision(c, s, p)
}
