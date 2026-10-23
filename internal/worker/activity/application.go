// Package activity: шаги процессов Temporal. Логика в usecase, activity его
// только вызывает; ошибки сети и 5xx возвращаем как есть, повторит RetryPolicy.
package activity

import (
	"context"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// ApplicationActivities шаги ApplicationWorkflow, которые трогают заявку.
type ApplicationActivities struct {
	apps usecase.ApplicationUseCase
}

func NewApplicationActivities(apps usecase.ApplicationUseCase) *ApplicationActivities {
	return &ApplicationActivities{apps: apps}
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
