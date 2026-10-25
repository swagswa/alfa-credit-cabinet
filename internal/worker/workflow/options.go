//nolint:mnd // здесь только таймауты и политики повторов
package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// DefaultRetryPolicy для вызовов нашей БД: сбой обычно короткий, рестарт или failover.
// Попытки не ограничиваем: с лимитом минута простоя Postgres роняла бы все идущие заявки,
// а доменные ошибки и так неповторяемые.
var DefaultRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2,
	MaximumInterval:    30 * time.Second,
}

// WithDBActivityOptions для быстрых шагов, которые пишут в Postgres.
func WithDBActivityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         DefaultRetryPolicy,
	})
}

// WithContractActivityOptions для договора: из accepted заявке деваться некуда, поэтому
// повторяем, пока не выйдет. Висит долго, смотреть в Temporal UI, это баг, а не сбой.
func WithContractActivityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    time.Minute,
		},
	})
}

// SourceRetryPolicy для внешних источников: мок может моргнуть или лежать долго,
// ждём около 15 с и уходим в ручное рассмотрение. На проде попыток больше.
var SourceRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2,
	MaximumInterval:    10 * time.Second,
	MaximumAttempts:    5,
}

// WithSourceActivityOptions для запросов в ЕГРЮЛ, ФНС, БКИ и данные банка.
func WithSourceActivityOptions(ctx workflow.Context) workflow.Context {
	return workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Second,
		RetryPolicy:         SourceRetryPolicy,
	})
}
