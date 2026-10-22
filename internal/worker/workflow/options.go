//nolint:mnd // здесь только таймауты и политики повторов
package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// DefaultRetryPolicy для вызовов нашей БД: сбой обычно короткий, рестарт или failover.
var DefaultRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2,
	MaximumInterval:    30 * time.Second,
	MaximumAttempts:    10,
}

// WithDBActivityOptions для быстрых шагов, которые пишут в Postgres.
func WithDBActivityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         DefaultRetryPolicy,
	})
}
