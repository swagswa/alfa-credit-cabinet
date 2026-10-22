package workflow_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"

	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/workflow"
)

func TestRecognizeWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		attempts int
		failed   bool
	}{
		{name: "распознали с первого раза", attempts: 1},
		{name: "ollama не поднялась", err: errors.New("connection refused"), attempts: 2, failed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s testsuite.WorkflowTestSuite
			env := s.NewTestWorkflowEnvironment()
			var a *activity.DocumentActivities
			doc := uuid.New()
			env.RegisterActivity(a)
			env.OnActivity(a.Recognize, mock.Anything, doc).Return(tt.err).Times(tt.attempts)
			if tt.failed {
				env.OnActivity(a.FailDocument, mock.Anything, doc, mock.Anything).Return(nil).Once()
			}

			env.ExecuteWorkflow(workflow.RecognizeWorkflow, doc)
			require.True(t, env.IsWorkflowCompleted())
			require.NoError(t, env.GetWorkflowError())
			env.AssertExpectations(t)
		})
	}
}
