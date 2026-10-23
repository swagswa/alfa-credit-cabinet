package workflow_test

import (
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

var system = domain.Actor{Role: domain.RoleSystem}

var toChecking = domain.Move{To: domain.StatusChecking, Actor: system}

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
	apps    *activity.ApplicationActivities
	checks  *activity.CheckActivities
	id      uuid.UUID
	fetches int // сколько раз должны спросить источники, с повторами
	called  int
}

func newEnv() *testEnv {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(5 * time.Second)
	env.RegisterWorkflow(workflow.ApplicationWorkflow)
	return &testEnv{env, &activity.ApplicationActivities{}, &activity.CheckActivities{}, uuid.New(), 0, 0}
}

func (e *testEnv) source(src domain.Source, b behavior) {
	fetch := func() *testsuite.MockCallWrapper {
		return e.OnActivity(e.checks.FetchSource, mock.Anything, e.id, src).Run(func(mock.Arguments) { e.called++ })
	}
	// чем источник ответил в последний раз: это и должно лечь в check_results
	failed := string(src) + ": 503"
	attempts := 1
	switch b {
	case answers:
		fetch().Return(domain.CheckOK, nil).Once()
	case notFound:
		fetch().Return(domain.CheckNotFound, nil).Once()
	case down:
		// без Times: считаем каждую попытку, сколько бы их ни было
		attempts = int(workflow.SourceRetryPolicy.MaximumAttempts)
		fetch().Return(domain.CheckStatus(""), temporal.NewApplicationError(failed, ""))
	case flaky:
		attempts = 3
		fetch().Return(domain.CheckStatus(""), temporal.NewApplicationError(failed, "")).Twice()
		fetch().Return(domain.CheckOK, nil).Once()
	case refuses:
		failed = string(src) + ": 403"
		err := temporal.NewNonRetryableApplicationError(failed, usecase.ForbiddenErrorType, nil)
		fetch().Return(domain.CheckStatus(""), err).Once()
	}
	e.fetches += attempts
	if b == down || b == refuses {
		e.OnActivity(e.checks.SaveUnavailable, mock.Anything, e.id, src, failed).Return(nil).Once()
	}
}

func TestApplicationWorkflow(t *testing.T) {
	needInfo := domain.Move{To: domain.StatusNeedInfo, Actor: system, Codes: []string{domain.CodeNotFound}}
	manual := domain.Move{To: domain.StatusManualReview, Actor: system, Codes: []string{domain.CodeSourceUnavailable}}

	tests := []struct {
		name    string
		sources map[domain.Source]behavior // кого нет, тот отвечает
		want    *domain.Move               // nil: заявка остаётся в checking до решения
	}{
		{"все ответили", nil, nil},
		{"нет кредитной истории это не повод", map[domain.Source]behavior{domain.SourceBKI: notFound}, nil},
		{"фнс ответила с третьей попытки", map[domain.Source]behavior{domain.SourceFNS: flaky}, nil},
		{"инн нет в егрюл", map[domain.Source]behavior{domain.SourceEGRUL: notFound}, &needInfo},
		{"фнс лежит", map[domain.Source]behavior{domain.SourceFNS: down}, &manual},
		{"бки без согласия", map[domain.Source]behavior{domain.SourceBKI: refuses}, &manual},
		{"запрос сильнее ручного", map[domain.Source]behavior{
			domain.SourceEGRUL: notFound, domain.SourceInternal: down,
		}, &needInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			e.OnActivity(e.apps.Transition, mock.Anything, e.id, toChecking).Return(nil).Once()
			for _, src := range domain.Sources {
				e.source(src, tt.sources[src])
			}
			if tt.want != nil {
				e.OnActivity(e.apps.Transition, mock.Anything, e.id, *tt.want).Return(nil).Once()
			}

			// запускаем по имени, как usecase: имя и функция не должны разойтись
			e.ExecuteWorkflow(usecase.ApplicationWorkflowName, e.id)

			require.True(t, e.IsWorkflowCompleted())
			require.NoError(t, e.GetWorkflowError())
			e.AssertExpectations(t)
			require.Equal(t, e.fetches, e.called, "запросов в источники с повторами")
		})
	}
}

// переход не по таблице повторять бесполезно: workflow падает, в источники не ходит
func TestApplicationWorkflowIllegal(t *testing.T) {
	e := newEnv()
	err := temporal.NewNonRetryableApplicationError("illegal", usecase.IllegalTransitionErrorType, nil)
	e.OnActivity(e.apps.Transition, mock.Anything, e.id, toChecking).Return(err).Once()

	e.ExecuteWorkflow(workflow.ApplicationWorkflow, e.id)

	require.True(t, e.IsWorkflowCompleted())
	require.Error(t, e.GetWorkflowError())
	e.AssertExpectations(t)
}

// сбой БД на переходе повторяет RetryPolicy, а не workflow
func TestApplicationWorkflowRetry(t *testing.T) {
	e := newEnv()
	e.OnActivity(e.apps.Transition, mock.Anything, e.id, toChecking).
		Return(temporal.NewApplicationError("db down", "")).Twice()
	e.OnActivity(e.apps.Transition, mock.Anything, e.id, toChecking).Return(nil).Once()
	for _, src := range domain.Sources {
		e.source(src, answers)
	}

	e.ExecuteWorkflow(workflow.ApplicationWorkflow, e.id)

	require.True(t, e.IsWorkflowCompleted())
	require.NoError(t, e.GetWorkflowError())
	e.AssertExpectations(t)
}
