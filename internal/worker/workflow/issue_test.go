package workflow_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// offered доводит заявку до решения d без людей
func (e *testEnv) offered(d domain.Decision) {
	e.expect(domain.StatusChecking, domain.RoleSystem)
	e.round(nil)
	e.decides(d)
}

// accepted ждёт принятие условий, договор на дату принятия и подписание
func (e *testEnv) accepted() {
	e.expect(domain.StatusAccepted, domain.RoleClient)
	e.OnActivity(e.apps.PrepareContract, mock.Anything, e.id, mock.MatchedBy(func(d time.Time) bool {
		return d.Equal(e.Now())
	})).Return(nil).Once()
	e.expect(domain.StatusSigning, domain.RoleSystem)
}

// issued ждёт выдачу и закрытие
func (e *testEnv) issued() {
	e.OnActivity(e.apps.Payout, mock.Anything, e.id).Return(nil).Once()
	e.expect(domain.StatusIssued, domain.RoleClient)
	e.expect(domain.StatusClosed, domain.RoleSystem)
}

func illegal(t *testing.T, err error) {
	t.Helper()
	appErr, ok := errors.AsType[*temporal.ApplicationError](err)
	require.True(t, ok, "ответ валидатора: %v", err)
	require.Equal(t, usecase.IllegalTransitionErrorType, appErr.Type())
}

func TestIssue(t *testing.T) {
	accept := domain.Action{Actor: client, Version: 7}
	sign := domain.Action{Actor: client}
	tests := []struct {
		name  string
		d     domain.Decision
		steps func(e *testEnv) []*reply
		took  time.Duration
	}{
		{"кофейня до closed", coffee, func(e *testEnv) []*reply {
			e.accepted()
			e.issued()
			return []*reply{
				e.act(time.Minute, usecase.UpdateAccept, accept),
				e.act(2*time.Minute, usecase.UpdateSign, sign),
			}
		}, 2*time.Minute + deadlines.Close},
		{"условия не приняли", coffee, func(e *testEnv) []*reply {
			e.expect(domain.StatusExpired, domain.RoleSystem)
			return nil
		}, deadlines.Offer},
		{"не подписали за срок", logistics, func(e *testEnv) []*reply {
			e.accepted()
			e.expect(domain.StatusExpired, domain.RoleSystem)
			return []*reply{e.act(time.Minute, usecase.UpdateAccept, accept)}
		}, time.Minute + deadlines.Signing},
		{"отказался от условий", logistics, func(e *testEnv) []*reply {
			e.expect(domain.StatusCancelledByClient, domain.RoleClient)
			return []*reply{e.act(time.Minute, usecase.UpdateCancel, domain.Action{Actor: client})}
		}, time.Minute},
		{"передумал на подписании", coffee, func(e *testEnv) []*reply {
			e.accepted()
			e.expect(domain.StatusCancelledByClient, domain.RoleClient)
			return []*reply{
				e.act(time.Minute, usecase.UpdateAccept, accept),
				e.act(2*time.Minute, usecase.UpdateCancel, domain.Action{Actor: client}),
			}
		}, 2 * time.Minute},
		{"подписал в последнюю секунду, выдача дольше срока", coffee, func(e *testEnv) []*reply {
			e.accepted()
			e.OnActivity(e.apps.Payout, mock.Anything, e.id).After(30 * time.Second).Return(nil).Once()
			e.expect(domain.StatusIssued, domain.RoleClient)
			e.expect(domain.StatusClosed, domain.RoleSystem)
			return []*reply{
				e.act(time.Minute, usecase.UpdateAccept, accept),
				e.act(time.Minute+deadlines.Signing-time.Second, usecase.UpdateSign, sign),
			}
		}, time.Minute + deadlines.Signing - time.Second + 30*time.Second + deadlines.Close},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.offered(tt.d)
			replies := tt.steps(e)

			require.Equal(t, tt.took, e.run())
			require.NoError(t, e.GetWorkflowError())
			for _, r := range replies {
				require.NoError(t, r.err)
			}
		})
	}
}

// карточка устарела: андеррайтер или процесс успели поменять условия
func TestAcceptStale(t *testing.T) {
	e := newEnv(t)
	e.offered(logistics)
	stale := temporal.NewNonRetryableApplicationError("заявка уже изменилась", usecase.VersionConflictErrorType, nil)
	e.OnActivity(e.apps.Transition, mock.Anything, e.id, mock.MatchedBy(func(m domain.Move) bool {
		return m.To == domain.StatusAccepted && m.Version == 3
	})).Return(stale).Once()
	first := e.act(time.Minute, usecase.UpdateAccept, domain.Action{Actor: client, Version: 3})
	e.accepted()
	second := e.act(2*time.Minute, usecase.UpdateAccept, domain.Action{Actor: client, Version: 4})
	e.RegisterDelayedCallback(e.CancelWorkflow, 3*time.Minute)

	e.run()
	appErr, ok := errors.AsType[*temporal.ApplicationError](first.err)
	require.True(t, ok, "ответ процесса: %v", first.err)
	require.Equal(t, usecase.VersionConflictErrorType, appErr.Type())
	require.NoError(t, second.err)
}

func TestIssueRejects(t *testing.T) {
	t.Run("подписать до принятия", func(t *testing.T) {
		e := newEnv(t)
		e.offered(coffee)
		got := e.act(time.Minute, usecase.UpdateSign, domain.Action{Actor: client})
		e.RegisterDelayedCallback(e.CancelWorkflow, 2*time.Minute)
		e.run()
		illegal(t, got.err)
	})
	t.Run("отмена, пока готовим договор", func(t *testing.T) {
		e := newEnv(t)
		e.offered(coffee)
		e.expect(domain.StatusAccepted, domain.RoleClient)
		e.OnActivity(e.apps.PrepareContract, mock.Anything, e.id, mock.Anything).After(time.Minute).Return(nil).Once()
		e.expect(domain.StatusSigning, domain.RoleSystem)
		e.act(time.Minute, usecase.UpdateAccept, domain.Action{Actor: client, Version: 7})
		got := e.act(time.Minute+time.Second, usecase.UpdateCancel, domain.Action{Actor: client})
		e.RegisterDelayedCallback(e.CancelWorkflow, 3*time.Minute)
		e.run()
		illegal(t, got.err)
	})
}

// Клиент видит approved в базе раньше, чем процесс получил ответ Transition:
// принятие ждёт конца перехода, а не отклоняется
func TestAcceptDuringMove(t *testing.T) {
	e := newEnv(t)
	e.expect(domain.StatusChecking, domain.RoleSystem)
	e.round(nil)
	e.OnActivity(e.apps.Decide, mock.Anything, e.id).Return(coffee, nil).Once()
	e.expect(domain.StatusScoring, domain.RoleSystem)
	e.move(coffee.Outcome, domain.RoleSystem, &coffee, coffee.Codes()...).After(10 * time.Second)
	e.accepted()
	e.expect(domain.StatusExpired, domain.RoleSystem)
	r := e.act(5*time.Second, usecase.UpdateAccept, domain.Action{Actor: client, Version: 7})

	e.run()
	require.NoError(t, e.GetWorkflowError())
	require.NoError(t, r.err)
}

// logistics подобрала свои условия под встречным: расчёт по ним уходит в базу вместе с принятием
func TestAcceptChoice(t *testing.T) {
	e := newEnv(t)
	e.offered(logistics)
	choice := logistics
	choice.Outcome = domain.StatusApproved
	choice.Terms = &domain.Terms{Product: domain.Equipment, AmountKop: 350_000_000, TermMonths: 48}
	e.move(domain.StatusAccepted, domain.RoleClient, &choice)
	e.OnActivity(e.apps.PrepareContract, mock.Anything, e.id, mock.Anything).Return(nil).Once()
	e.expect(domain.StatusSigning, domain.RoleSystem)
	r := e.act(time.Minute, usecase.UpdateAccept, domain.Action{Actor: client, Version: 7, Decision: &choice})
	e.RegisterDelayedCallback(e.CancelWorkflow, 2*time.Minute)

	e.run()
	require.NoError(t, r.err)
}
