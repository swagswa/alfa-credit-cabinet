package workflow_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/workflow"
)

var deadlines = domain.Deadlines{
	NeedInfo: 3 * time.Minute, Review: 5 * time.Minute, Offer: 5 * time.Minute, Signing: 5 * time.Minute,
	Close: time.Minute,
}

var (
	client      = domain.Actor{ID: uuid.New(), Role: domain.RoleClient}
	manager     = domain.Actor{ID: uuid.New(), Role: domain.RoleManager}
	underwriter = domain.Actor{ID: uuid.New(), Role: domain.RoleUnderwriter}
)

// как источник ведёт себя в сценарии
type behavior int

const (
	answers  behavior = iota
	notFound          // 404
	down              // 503 на всех попытках
	flaky             // 503 дважды, потом ответ
	refuses           // 403, повторять бесполезно
)

type testEnv struct {
	*testsuite.TestWorkflowEnvironment
	t      *testing.T
	apps   *activity.ApplicationActivities
	checks *activity.CheckActivities
	id     uuid.UUID
	extra  int // запросы в источники сверх сценария
	start  time.Time
}

func newEnv(t *testing.T) *testEnv {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(5 * time.Second)
	env.RegisterWorkflow(workflow.ApplicationWorkflow)
	e := &testEnv{
		TestWorkflowEnvironment: env, t: t, id: uuid.New(), start: env.Now(),
		apps: &activity.ApplicationActivities{}, checks: &activity.CheckActivities{},
	}
	e.OnActivity(e.apps.Deadlines, mock.Anything).Return(deadlines, nil).Once()
	return e
}

// expect ждёт переход в to от роли с такими кодами и без решения.
func (e *testEnv) expect(to domain.Status, role domain.Role, codes ...string) *testsuite.MockCallWrapper {
	return e.move(to, role, nil, codes...)
}

// move ждёт переход с решением d. У шагов, где ждут людей, срок считается от
// момента перехода, у остальных его нет.
func (e *testEnv) move(
	to domain.Status,
	role domain.Role,
	d *domain.Decision,
	codes ...string,
) *testsuite.MockCallWrapper {
	ttl := map[domain.Status]time.Duration{
		domain.StatusNeedInfo:     deadlines.NeedInfo,
		domain.StatusManualReview: deadlines.Review,
		domain.StatusApproved:     deadlines.Offer,
		domain.StatusCounterOffer: deadlines.Offer,
		domain.StatusSigning:      deadlines.Signing,
	}
	return e.OnActivity(e.apps.Transition, mock.Anything, e.id, mock.MatchedBy(func(m domain.Move) bool {
		ttl, waits := ttl[to]
		due := m.DueAt == nil
		if waits {
			due = m.DueAt != nil && m.DueAt.Equal(e.Now().Add(ttl))
		}
		return m.To == to && m.Actor.Role == role && slices.Equal(m.Codes, codes) && due &&
			reflect.DeepEqual(m.Decision, d)
	})).Return(nil).Once()
}

// decides отдаёт решение d из расчёта и ждёт, что заявка пойдёт в его исход:
// после стоп-факторов сразу, после политики через scoring.
func (e *testEnv) decides(d domain.Decision) {
	e.OnActivity(e.apps.Decide, mock.Anything, e.id).Return(d, nil).Once()
	if d.Stage == domain.StagePolicy {
		e.expect(domain.StatusScoring, domain.RoleSystem)
	}
	e.move(d.Outcome, domain.RoleSystem, &d, d.Codes()...)
}

// approves: расчёт одобрил, дальше клиент условия так и не принял
func (e *testEnv) approves() {
	e.decides(coffee)
	e.expect(domain.StatusExpired, domain.RoleSystem)
}

func (e *testEnv) round(sources map[domain.Source]behavior) {
	for _, src := range domain.Sources {
		e.source(src, sources[src])
	}
}

func (e *testEnv) source(src domain.Source, b behavior) {
	fetch := func() *testsuite.MockCallWrapper {
		return e.OnActivity(e.checks.FetchSource, mock.Anything, e.id, src)
	}
	// чем источник ответил в последний раз: это и должно лечь в check_results
	failed := string(src) + ": 503"
	switch b {
	case answers:
		fetch().Return(domain.CheckOK, nil).Once()
	case notFound:
		fetch().Return(domain.CheckNotFound, nil).Once()
	case down:
		fetch().Return(domain.CheckStatus(""), temporal.NewApplicationError(failed, "")).
			Times(5) // пять попыток по модели процесса
	case flaky:
		fetch().Return(domain.CheckStatus(""), temporal.NewApplicationError(failed, "")).Twice()
		fetch().Return(domain.CheckOK, nil).Once()
	case refuses:
		failed = string(src) + ": 403"
		err := temporal.NewNonRetryableApplicationError(failed, usecase.ForbiddenErrorType, nil)
		fetch().Return(domain.CheckStatus(""), err).Once()
	}
	if b == down || b == refuses {
		e.OnActivity(e.checks.SaveUnavailable, mock.Anything, e.id, src, failed).Return(nil).Once()
	}
}

// reply это ответ процесса на действие: nil принято и исполнено,
// иначе отказ валидатора или ошибка обработчика.
type reply struct{ err error }

// act шлёт действие человека в момент at после старта.
func (e *testEnv) act(at time.Duration, update string, a domain.Action) *reply {
	r := &reply{}
	e.RegisterDelayedCallback(func() {
		e.UpdateWorkflow(update, "", &testsuite.TestUpdateCallback{
			OnAccept:   func() {},
			OnReject:   func(err error) { r.err = err },
			OnComplete: func(_ any, err error) { r.err = err },
		}, a)
	}, at)
	return r
}

// run запускает процесс по имени, как usecase, и отдаёт, сколько времени он шёл.
// Times в моке ловит только недобор вызовов. Лишние попытки уходят в ловушку,
// она стоит последней и срабатывает, когда сценарные ожидания исчерпаны.
func (e *testEnv) run() time.Duration {
	e.OnActivity(e.checks.FetchSource, mock.Anything, mock.Anything, mock.Anything).
		Run(func(mock.Arguments) { e.extra++ }).
		Return(domain.CheckStatus(""), temporal.NewNonRetryableApplicationError("лишний запрос", "", nil)).
		Maybe()
	e.ExecuteWorkflow(usecase.ApplicationWorkflowName, e.id)
	require.True(e.t, e.IsWorkflowCompleted())
	e.AssertExpectations(e.t)
	require.Zero(e.t, e.extra, "лишние запросы в источники")
	return e.Now().Sub(e.start)
}

func TestChecks(t *testing.T) {
	tests := []struct {
		name    string
		sources map[domain.Source]behavior // кого нет, тот отвечает
		to      domain.Status              // пусто: заявка остаётся в checking до решения
		code    string
	}{
		{"все ответили", nil, "", ""},
		{"егрюл лежит", map[domain.Source]behavior{domain.SourceEGRUL: down},
			domain.StatusManualReview, domain.CodeSourceUnavailable},
		{"нет кредитной истории это не повод", map[domain.Source]behavior{domain.SourceBKI: notFound}, "", ""},
		{"фнс ответила с третьей попытки", map[domain.Source]behavior{domain.SourceFNS: flaky}, "", ""},
		{"инн нет в егрюл", map[domain.Source]behavior{domain.SourceEGRUL: notFound},
			domain.StatusNeedInfo, domain.CodeNotFound},
		{"фнс лежит", map[domain.Source]behavior{domain.SourceFNS: down},
			domain.StatusManualReview, domain.CodeSourceUnavailable},
		{"бки без согласия", map[domain.Source]behavior{domain.SourceBKI: refuses},
			domain.StatusManualReview, domain.CodeSourceUnavailable},
		{"запрос сильнее ручного", map[domain.Source]behavior{
			domain.SourceEGRUL: notFound, domain.SourceInternal: down,
		}, domain.StatusNeedInfo, domain.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.expect(domain.StatusChecking, domain.RoleSystem)
			e.round(tt.sources)
			if tt.to != "" {
				e.expect(tt.to, domain.RoleSystem, tt.code)
				// дальше заявка ждёт людей, это другие тесты
				e.RegisterDelayedCallback(e.CancelWorkflow, time.Minute)
			} else {
				e.approves()
			}

			e.run()
			if tt.to == "" {
				require.NoError(t, e.GetWorkflowError())
			}
		})
	}
}

func TestNeedInfo(t *testing.T) {
	notInEGRUL := map[domain.Source]behavior{domain.SourceEGRUL: notFound}
	stale := temporal.NewNonRetryableApplicationError("old", usecase.VersionConflictErrorType, nil)

	t.Run("клиент ответил, проверки заново", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(notInEGRUL)
		e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
		got := e.act(time.Minute, usecase.UpdateProvideInfo, domain.Action{Actor: client, Version: 3})
		e.expect(domain.StatusChecking, domain.RoleClient)
		e.round(nil)
		e.approves()

		e.run()
		require.NoError(t, e.GetWorkflowError())
		require.NoError(t, got.err)
	})

	t.Run("клиент молчит", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(notInEGRUL)
		e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
		e.expect(domain.StatusExpired, domain.RoleSystem)

		elapsed := e.run()
		require.NoError(t, e.GetWorkflowError())
		require.Equal(t, deadlines.NeedInfo, elapsed.Truncate(time.Minute), "истекает ровно по сроку")
	})

	t.Run("ответ за секунду до срока не истекает", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(notInEGRUL)
		e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
		got := e.act(
			deadlines.NeedInfo-time.Second,
			usecase.UpdateProvideInfo,
			domain.Action{Actor: client, Version: 3},
		)
		// переход долгий, срок выходит посреди него
		e.expect(domain.StatusChecking, domain.RoleClient).After(5 * time.Second)
		// а это второе нажатие, пока первое ещё идёт
		late := e.act(deadlines.NeedInfo, usecase.UpdateCancel, domain.Action{Actor: client})
		e.round(nil)
		e.approves()

		e.run()
		require.NoError(t, e.GetWorkflowError())
		require.NoError(t, got.err)
		appErr, ok := errors.AsType[*temporal.ApplicationError](late.err)
		require.True(t, ok, "второе действие: %v", late.err)
		require.Equal(t, usecase.IllegalTransitionErrorType, appErr.Type())
	})

	t.Run("клиент отменил", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(notInEGRUL)
		e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
		got := e.act(time.Minute, usecase.UpdateCancel, domain.Action{Actor: client, Comment: "передумал"})
		e.expect(domain.StatusCancelledByClient, domain.RoleClient)

		e.run()
		require.NoError(t, e.GetWorkflowError())
		require.NoError(t, got.err)
	})

	t.Run("карточка устарела, срок идёт дальше", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(notInEGRUL)
		e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
		got := e.act(time.Minute, usecase.UpdateProvideInfo, domain.Action{Actor: client, Version: 1})
		e.expect(domain.StatusChecking, domain.RoleClient).Return(stale)
		e.expect(domain.StatusExpired, domain.RoleSystem)

		e.run()
		require.NoError(t, e.GetWorkflowError())
		appErr, ok := errors.AsType[*temporal.ApplicationError](got.err)
		require.True(t, ok, "ответ обработчика: %v", got.err)
		require.Equal(t, usecase.VersionConflictErrorType, appErr.Type())
	})
}

func TestManualReview(t *testing.T) {
	fnsDown := map[domain.Source]behavior{domain.SourceFNS: down}

	t.Run("sla прошёл, менеджер повторил проверки", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(fnsDown)
		e.expect(domain.StatusManualReview, domain.RoleSystem, domain.CodeSourceUnavailable)
		e.OnActivity(e.apps.Escalate, mock.Anything, e.id).Return(nil).Once()
		got := e.act(10*time.Minute, usecase.UpdateRetryChecks, domain.Action{Actor: manager, Version: 3})
		e.expect(domain.StatusChecking, domain.RoleManager)
		e.round(nil)
		e.approves()

		e.run()
		require.NoError(t, e.GetWorkflowError())
		require.NoError(t, got.err)
	})

	t.Run("андеррайтер запросил документы", func(t *testing.T) {
		e := newEnv(t)
		e.expect(domain.StatusChecking, domain.RoleSystem)
		e.round(fnsDown)
		e.expect(domain.StatusManualReview, domain.RoleSystem, domain.CodeSourceUnavailable)
		a := domain.Action{Actor: underwriter, Version: 3, Comment: "выписка за 12 месяцев"}
		got := e.act(time.Minute, usecase.UpdateRequestInfo, a)
		e.expect(domain.StatusNeedInfo, domain.RoleUnderwriter)
		e.expect(domain.StatusExpired, domain.RoleSystem)

		e.run()
		require.NoError(t, e.GetWorkflowError())
		require.NoError(t, got.err)
	})
}

// пока идут проверки, перейти нельзя: запрос исполняется сразу после них
func TestRequestInfoWhileChecking(t *testing.T) {
	e := newEnv(t)
	e.expect(domain.StatusChecking, domain.RoleSystem)
	e.round(map[domain.Source]behavior{domain.SourceFNS: flaky})
	got := e.act(time.Second, usecase.UpdateRequestInfo, domain.Action{Actor: manager, Comment: "уточните выручку"})
	e.expect(domain.StatusNeedInfo, domain.RoleManager)
	e.expect(domain.StatusExpired, domain.RoleSystem)

	e.run()
	require.NoError(t, e.GetWorkflowError())
	require.NoError(t, got.err)
}

func TestActionRejected(t *testing.T) {
	tests := []struct {
		name   string
		update string
		actor  domain.Actor
	}{
		{"клиент повторяет проверки", usecase.UpdateRetryChecks, client},
		{"менеджер отвечает за клиента", usecase.UpdateProvideInfo, manager},
		{"запрос документов, когда их уже ждут", usecase.UpdateRequestInfo, manager},
		{"андеррайтер отменяет", usecase.UpdateCancel, underwriter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.expect(domain.StatusChecking, domain.RoleSystem)
			e.round(map[domain.Source]behavior{domain.SourceEGRUL: notFound})
			e.expect(domain.StatusNeedInfo, domain.RoleSystem, domain.CodeNotFound)
			got := e.act(time.Minute, tt.update, domain.Action{Actor: tt.actor})
			e.expect(domain.StatusExpired, domain.RoleSystem)

			e.run()
			require.NoError(t, e.GetWorkflowError())
			appErr, ok := errors.AsType[*temporal.ApplicationError](got.err)
			require.True(t, ok, "ответ валидатора: %v", got.err)
			require.Equal(t, usecase.IllegalTransitionErrorType, appErr.Type())
		})
	}
}

// переход не по таблице повторять бесполезно: workflow падает, в источники не ходит
func TestApplicationWorkflowIllegal(t *testing.T) {
	e := newEnv(t)
	err := temporal.NewNonRetryableApplicationError("illegal", usecase.IllegalTransitionErrorType, nil)
	e.expect(domain.StatusChecking, domain.RoleSystem).Return(err)

	e.run()
	require.Error(t, e.GetWorkflowError())
}

// сбой БД на переходе повторяет RetryPolicy, а не workflow
func TestApplicationWorkflowRetry(t *testing.T) {
	e := newEnv(t)
	e.expect(domain.StatusChecking, domain.RoleSystem).Return(temporal.NewApplicationError("db down", "")).Twice()
	e.expect(domain.StatusChecking, domain.RoleSystem)
	e.round(nil)
	e.approves()

	e.run()
	require.NoError(t, e.GetWorkflowError())
}
