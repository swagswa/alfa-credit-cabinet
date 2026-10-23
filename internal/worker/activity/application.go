// Package activity: шаги процессов Temporal. Логика в usecase, activity его
// только вызывает; ошибки сети и 5xx возвращаем как есть, повторит RetryPolicy.
package activity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// Типы неповторяемых ошибок: workflow по ним понимает, что повтор не поможет.
const (
	NotFoundErrorType          = "NotFoundError"
	IllegalTransitionErrorType = "IllegalTransitionError"
	VersionConflictErrorType   = "VersionConflictError"
	ForbiddenErrorType         = "ForbiddenError"
	InvalidErrorType           = "InvalidError"
)

var nonRetryable = []struct {
	err     error
	errType string
}{
	{domain.ErrNotFound, NotFoundErrorType},
	{domain.ErrIllegalTransition, IllegalTransitionErrorType},
	{domain.ErrConflict, VersionConflictErrorType},
	{domain.ErrForbidden, ForbiddenErrorType},
	{domain.ErrInvalid, InvalidErrorType},
}

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

func classify(err error) error {
	for _, nr := range nonRetryable {
		if errors.Is(err, nr.err) {
			return temporal.NewNonRetryableApplicationError(err.Error(), nr.errType, err)
		}
	}
	return err
}
