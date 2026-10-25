package rules_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
)

const policyPath = "../../rules/policy.yaml"

func TestPolicy(t *testing.T) {
	p, err := rules.Policy(policyPath)
	require.NoError(t, err)

	require.InDelta(t, 0.25, p.DSRMax["C"], 1e-9)
	require.Equal(t, 150, p.RatePremiumBP["B"])
	require.Equal(t, int64(1_000_000), p.LimitRoundKop)

	wc := p.Products[domain.WorkingCapital]
	require.Equal(t, int64(500_000_000), wc.AutoLimitKop)
	require.Equal(t, int64(150_000_000), *wc.Grades["C"].GuarantorOverKop)
	require.Equal(t, int64(30_000_000), *wc.Grades["D"].GuarantorOverKop)
	require.Nil(t, wc.Grades["A"].GuarantorOverKop)
	require.Equal(t, domain.GradeReject, wc.Grades["E"].Action)
	require.InDelta(t, 0.4, p.Products[domain.Overdraft].Grades["B"].TurnoverShare, 1e-9)

	require.Len(t, p.StopFactors, 17)
	require.Equal(t, domain.StopRule{
		Code:    "SF_STATUS_INACTIVE",
		Input:   "egrul_status",
		Op:      domain.OpNe,
		Value:   "active",
		Outcome: domain.StatusRejected,
	}, p.StopFactors[0])
	require.Equal(t, domain.StopRule{
		Code: "SF_AGE_MIN", Input: "business_age_months", Op: domain.OpLt, Value: 6.0, Outcome: domain.StatusRejected,
	}, p.StopFactors[2])
	require.Equal(t, "выписка по счёту из другого банка", p.DocumentNames["statement"])
	require.Equal(t, "Не хватает документа: {doc}", p.Texts["SF_DOCS_MISSING"])
}

func TestPolicyBroken(t *testing.T) {
	tests := []struct{ name, old, repl string }{
		{"опечатка в поле", "limit_round_rub: 10000", "limit_round: 10000"},
		{"неизвестный input", "input: mass_address", "input: mass_addr"},
		{"два оператора", "ne: active, outcome", "ne: active, eq: active, outcome"},
		{"без оператора", "ne: active, outcome", "outcome"},
		{"gt на строке", "ne: active, outcome", "gt: active, outcome"},
		{"gt на bool", "input: bankruptcy, eq: true", "input: bankruptcy, gt: true"},
		{"тип значения", "input: bankruptcy, eq: true", "input: bankruptcy, eq: 1"},
		{"неизвестный исход", "lt: 6, outcome: reject", "lt: 6, outcome: decline"},
		{"нет класса E", "      E: {action: reject}\n  overdraft:", "  overdraft:"},
		{"неизвестное действие", "A: {action: auto, revenue_mult: 4", "A: {action: maybe, revenue_mult: 4"},
		{"кредит без срока", "revenue_mult: 4, max_term: 36", "revenue_mult: 4"},
		{"неизвестный продукт", "  equipment:", "  leasing:"},
		{"dsr больше 1", "C: 0.25, D", "C: 1.25, D"},
		{"нулевой лимит авторешения", "auto_limit_rub: 5000000", "auto_limit_rub: 0"},
		{"доля переключения 0", "switch_age_min_share: 0.3", "switch_age_min_share: 0"},
		{"нет dsr у класса", "dsr_max: {A: 0.30, B: 0.30, C: 0.25, D: 0.25}", "dsr_max: {A: 0.30, B: 0.30, C: 0.25}"},
		{"нет надбавки у класса", "rate_premium_pp: {A: 0, B: 1.5, C: 3.5, D: 6}", "rate_premium_pp: {A: 0, B: 1.5}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rules.Policy(broken(t, policyPath, tt.old, tt.repl))
			require.Error(t, err)
		})
	}
}
