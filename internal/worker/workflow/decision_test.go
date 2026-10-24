package workflow_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

const mln = 1_000_000 * domain.KopInRub

// Расчёты демо-компаний из testdata/scoring_cases.yaml в том виде, в каком их отдаёт activity.
// Цифры и коды тут для узнаваемости, сам расчёт проверяют тесты usecase и activity.
var (
	coffee = domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusApproved,
		Score: &domain.Score{Points: 87, Grade: "A"},
		Terms: &domain.Terms{
			Product: domain.WorkingCapital, AmountKop: 2.5 * mln, TermMonths: 24, RateBP: 1900, PaymentKop: 126_020_00,
		},
		Reasons: reasons("R_APPROVED", "SC_NO_OVERDUE", "SC_DSR_LOW"),
	}
	logistics = domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusCounterOffer,
		Score: &domain.Score{Points: 64, Grade: "C"},
		Terms: &domain.Terms{
			Product: domain.Equipment, AmountKop: 3.9 * mln, TermMonths: 48, RateBP: 2050, PaymentKop: 117_000_00,
			Pledge: true,
		},
		Reasons: reasons("R_TERM_EXTENDED", "R_AMOUNT_DSR", "R_PLEDGE", "R_RATE_PREMIUM"),
	}
	build = domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusRejected,
		Score:   &domain.Score{Points: 27, Grade: "E"},
		Reasons: reasons("R_GRADE_E", "SC_REVENUE_DROP", "SC_DSR_HIGH"),
	}
	overdue = domain.Decision{
		Stage: domain.StageChecks, Outcome: domain.StatusRejected,
		Hits:    []domain.StopHit{{Code: "SF_OVERDUE_90_ACTIVE", Outcome: domain.StatusRejected}},
		Reasons: reasons("SF_OVERDUE_90_ACTIVE"),
	}
	itDocs = domain.Decision{
		Stage: domain.StageChecks, Outcome: domain.StatusNeedInfo,
		Hits:    []domain.StopHit{{Code: "SF_DOCS_MISSING", Outcome: domain.StatusNeedInfo}},
		Reasons: reasons("SF_DOCS_MISSING"),
	}
	itTerms = domain.Terms{
		Product: domain.WorkingCapital, AmountKop: 7 * mln, TermMonths: 12, RateBP: 2250, PaymentKop: 656_000_00,
		Guarantor: true,
	}
	itReview = domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusManualReview,
		Score:          &domain.Score{Points: 60, Grade: "C"},
		Recommendation: &itTerms,
		Reasons:        reasons("R_MANUAL_AMOUNT", "R_AMOUNT_MULT", "R_GUARANTOR", "R_RATE_PREMIUM"),
	}
)

func reasons(codes ...string) []domain.Reason {
	out := make([]domain.Reason, 0, len(codes))
	for _, c := range codes {
		out = append(out, domain.Reason{Code: c, Text: "текст " + c})
	}
	return out
}

func TestDecision(t *testing.T) {
	tests := []struct {
		name string
		d    domain.Decision
	}{
		{"кофейня одобрена", coffee},
		{"логистике контрпредложение", logistics},
		{"стройке отказ по классу", build},
		{"просрочка 90+ отказ без скоринга", overdue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.expect(domain.StatusChecking, domain.RoleSystem)
			e.round(nil)
			e.decides(tt.d)
			if tt.d.Terms != nil {
				// условия так и не приняли
				e.expect(domain.StatusExpired, domain.RoleSystem)
			}

			e.run()
			require.NoError(t, e.GetWorkflowError())
		})
	}
}

// IT: нет документов, клиент догрузил, сумма выше авторешения, андеррайтер решает
func TestUnderwriter(t *testing.T) {
	offer := itTerms
	offer.AmountKop, offer.PaymentKop = 6*mln, 562_000_00
	verdict := func(outcome domain.Status, terms *domain.Terms, override bool, codes ...string) *domain.Decision {
		d := itReview
		d.Stage, d.Outcome, d.Terms, d.Override = domain.StageUnderwriter, outcome, terms, override
		d.Reasons = []domain.Reason{}
		for _, c := range codes {
			i := slices.IndexFunc(itReview.Reasons, func(r domain.Reason) bool { return r.Code == c })
			d.Reasons = append(d.Reasons, itReview.Reasons[i])
		}
		return &d
	}
	tests := []struct {
		name string
		act  domain.Action
		want *domain.Decision // nil: валидатор отклоняет, заявка ждёт дальше
	}{
		{
			"контрпредложение по рекомендации",
			domain.Action{
				Outcome: domain.StatusCounterOffer, Terms: &itTerms, Codes: []string{"R_AMOUNT_MULT", "R_GUARANTOR"},
				Comment: "контракт с госзаказчиком подтверждён",
			},
			verdict(domain.StatusCounterOffer, &itTerms, false, "R_AMOUNT_MULT", "R_GUARANTOR"),
		},
		{
			"меньше рекомендации это override",
			domain.Action{Outcome: domain.StatusCounterOffer, Terms: &offer, Comment: "сезонность, режу сумму"},
			verdict(domain.StatusCounterOffer, &offer, true),
		},
		{
			"отказ вопреки рекомендации, причина без текста в расчёте",
			domain.Action{Outcome: domain.StatusRejected, Codes: []string{"R_GRADE_E"}, Comment: "аффилированность"},
			func() *domain.Decision {
				d := verdict(domain.StatusRejected, nil, true)
				d.Reasons = []domain.Reason{{Code: "R_GRADE_E"}}
				return d
			}(),
		},
		{"без комментария", domain.Action{Outcome: domain.StatusApproved, Terms: &itTerms}, nil},
		{"одобрение без условий", domain.Action{Outcome: domain.StatusApproved, Comment: "ок"}, nil},
		{"решение это не запрос", domain.Action{Outcome: domain.StatusNeedInfo, Comment: "нужна выписка"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.expect(domain.StatusChecking, domain.RoleSystem)
			e.round(nil)
			e.decides(itDocs)
			info := e.act(time.Minute, usecase.UpdateProvideInfo, domain.Action{Actor: client, Version: 4})
			e.expect(domain.StatusChecking, domain.RoleClient)
			e.round(nil)
			e.decides(itReview)

			tt.act.Actor = underwriter
			got := e.act(3*time.Minute, usecase.UpdateDecide, tt.act)
			if tt.want != nil {
				e.move(tt.want.Outcome, domain.RoleUnderwriter, tt.want, tt.act.Codes...)
				if tt.want.Terms != nil {
					e.expect(domain.StatusExpired, domain.RoleSystem)
				}
			} else {
				// SLA прошёл, эскалация, а заявка так и ждёт специалиста
				e.OnActivity(e.apps.Escalate, mock.Anything, e.id).Return(nil).Once()
				e.RegisterDelayedCallback(e.CancelWorkflow, time.Hour)
			}

			e.run()
			require.NoError(t, info.err)
			if tt.want != nil {
				require.NoError(t, e.GetWorkflowError())
				require.NoError(t, got.err)
				return
			}
			appErr, ok := errors.AsType[*temporal.ApplicationError](got.err)
			require.True(t, ok, "ответ валидатора: %v", got.err)
			require.Equal(t, usecase.IllegalTransitionErrorType, appErr.Type())
		})
	}

	t.Run("менеджер не решает", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(nil)
		e.decides(itReview)
		a := domain.Action{Actor: manager, Outcome: domain.StatusApproved, Terms: &itTerms, Comment: "одобряю"}
		got := e.act(time.Minute, usecase.UpdateDecide, a)
		e.RegisterDelayedCallback(e.CancelWorkflow, 2*time.Minute)

		e.run()
		appErr, ok := errors.AsType[*temporal.ApplicationError](got.err)
		require.True(t, ok, "ответ валидатора: %v", got.err)
		require.Equal(t, usecase.IllegalTransitionErrorType, appErr.Type())
	})
}

// клиент так и не догрузил документы
func TestDocsNeverCame(t *testing.T) {
	e := newEnv(t)
	e.expect(domain.StatusChecking, domain.RoleSystem)
	e.round(nil)
	e.decides(itDocs)
	e.expect(domain.StatusExpired, domain.RoleSystem)

	require.Equal(t, deadlines.NeedInfo, e.run().Truncate(time.Minute))
	require.NoError(t, e.GetWorkflowError())
}
