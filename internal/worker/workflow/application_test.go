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

var toChecking = domain.Move{To: domain.StatusChecking, Actor: domain.Actor{Role: domain.RoleSystem}}

func newEnv() (*testsuite.TestWorkflowEnvironment, *activity.ApplicationActivities) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetTestTimeout(5 * time.Second)
	env.RegisterWorkflow(workflow.ApplicationWorkflow)
	return env, &activity.ApplicationActivities{}
}

func TestApplicationWorkflow(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"проверка началась", nil, false},
		// переход не по таблице повторять бесполезно, workflow падает сразу
		{"заявка не в submitted", temporal.NewNonRetryableApplicationError(
			"illegal", activity.IllegalTransitionErrorType, nil), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, a := newEnv()
			id := uuid.New()
			env.OnActivity(a.Transition, mock.Anything, id, toChecking).Return(tt.err).Once()

			// запускаем по имени, как usecase: имя и функция не должны разойтись
			env.ExecuteWorkflow(usecase.ApplicationWorkflowName, id)

			require.True(t, env.IsWorkflowCompleted())
			require.Equal(t, tt.wantErr, env.GetWorkflowError() != nil, "ошибка workflow: %v", env.GetWorkflowError())
			env.AssertExpectations(t)
		})
	}
}

// сбой БД повторяет RetryPolicy, а не workflow
func TestApplicationWorkflowRetry(t *testing.T) {
	env, a := newEnv()
	id := uuid.New()
	env.OnActivity(a.Transition, mock.Anything, id, toChecking).
		Return(temporal.NewApplicationError("db down", "")).
		Twice()
	env.OnActivity(a.Transition, mock.Anything, id, toChecking).Return(nil).Once()

	env.ExecuteWorkflow(workflow.ApplicationWorkflow, id)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	env.AssertExpectations(t)
}
