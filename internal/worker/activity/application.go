// Package activity: шаги процессов Temporal. Логика в usecase, activity его
// только вызывает; ошибки сети и 5xx возвращаем как есть, повторит RetryPolicy.
package activity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// ApplicationActivities шаги ApplicationWorkflow, которые трогают заявку.
type ApplicationActivities struct {
	apps      usecase.ApplicationUseCase
	decisions usecase.DecisionUseCase
	deadlines domain.Deadlines
}

func NewApplicationActivities(
	apps usecase.ApplicationUseCase,
	decisions usecase.DecisionUseCase,
	deadlines domain.Deadlines,
) *ApplicationActivities {
	return &ApplicationActivities{apps: apps, decisions: decisions, deadlines: deadlines}
}

// Deadlines отдаёт сроки из конфига воркера. Процесс берёт их один раз, и они
// ложатся в историю: смена конфига не ломает реплей идущих заявок.
func (a *ApplicationActivities) Deadlines(context.Context) (domain.Deadlines, error) {
	return a.deadlines, nil
}

// Decide считает решение по последним проверкам и ничего не пишет: решение ложится
// в базу вместе с переходом, поэтому повтор безопасен.
func (a *ApplicationActivities) Decide(ctx context.Context, id uuid.UUID) (domain.Decision, error) {
	app, err := a.apps.Get(ctx, domain.Actor{Role: domain.RoleSystem}, id)
	if err != nil {
		return domain.Decision{}, classify(err)
	}
	return a.decisions.Decide(&app, time.Now())
}

func (a *ApplicationActivities) Escalate(ctx context.Context, id uuid.UUID) error {
	return classify(a.apps.Escalate(ctx, id))
}

// Transition повторять безопасно: повтор после коммита ничего не меняет.
func (a *ApplicationActivities) Transition(ctx context.Context, id uuid.UUID, m domain.Move) error {
	return classify(a.apps.Transition(ctx, id, m))
}

// classify помечает ошибки, которые повторять бесполезно, остальные повторит RetryPolicy.
func classify(err error) error {
	if typ := usecase.ErrorType(err); typ != "" {
		return temporal.NewNonRetryableApplicationError(err.Error(), typ, err)
	}
	return err
}
