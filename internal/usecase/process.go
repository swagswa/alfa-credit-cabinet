package usecase

import (
	"errors"
	"fmt"

	"go.temporal.io/sdk/temporal"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

// Очереди, процессы и действия людей в Temporal. Живут здесь, а не в worker/workflow:
// процессы стартует и дёргает usecase, а workflow сам зовёт usecase через activity.
const (
	QueueCredit = "credit"
	QueueDocs   = "docs"

	ApplicationWorkflowName = "ApplicationWorkflow"

	UpdateProvideInfo = "ProvideInfo"
	UpdateRequestInfo = "RequestInfo"
	UpdateRetryChecks = "RetryChecks"
	UpdateCancel      = "Cancel"
)

// Типы ошибок, которыми процесс отвечает на шаг или действие человека. Повторять их бесполезно.
const (
	NotFoundErrorType          = "NotFoundError"
	IllegalTransitionErrorType = "IllegalTransitionError"
	VersionConflictErrorType   = "VersionConflictError"
	ForbiddenErrorType         = "ForbiddenError"
	InvalidErrorType           = "InvalidError"
)

var errorTypes = []struct {
	err error
	typ string
}{
	{domain.ErrNotFound, NotFoundErrorType},
	{domain.ErrIllegalTransition, IllegalTransitionErrorType},
	{domain.ErrConflict, VersionConflictErrorType},
	{domain.ErrForbidden, ForbiddenErrorType},
	{domain.ErrInvalid, InvalidErrorType},
}

// ErrorType отдаёт тип для доменной ошибки. Пустая строка: ошибку стоит повторить.
func ErrorType(err error) string {
	for _, t := range errorTypes {
		if errors.Is(err, t.err) {
			return t.typ
		}
	}
	return ""
}

// fromProcess переводит ответ процесса обратно в доменную ошибку для API.
func fromProcess(err error) error {
	if appErr, ok := errors.AsType[*temporal.ApplicationError](err); ok {
		for _, t := range errorTypes {
			if appErr.Type() == t.typ {
				return fmt.Errorf("%w: %s", t.err, appErr.Message())
			}
		}
	}
	if errors.Is(err, temporalsvc.ErrNoProcess) {
		return fmt.Errorf("%w: заявка сейчас не ждёт действий", domain.ErrIllegalTransition)
	}
	return err
}
