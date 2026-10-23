package activity

import (
	"context"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// CheckActivities запросы в источники проверок заявки.
type CheckActivities struct {
	checks usecase.CheckUseCase
}

func NewCheckActivities(checks usecase.CheckUseCase) *CheckActivities {
	return &CheckActivities{checks: checks}
}

// FetchSource спрашивает источник. Недоступный отдаёт ошибку, её повторяет RetryPolicy.
// 404 это не ошибка, а ответ not_found.
func (a *CheckActivities) FetchSource(
	ctx context.Context,
	id uuid.UUID,
	src domain.Source,
) (domain.CheckStatus, error) {
	status, err := a.checks.Fetch(ctx, id, src)
	return status, classify(err)
}

func (a *CheckActivities) SaveUnavailable(ctx context.Context, id uuid.UUID, src domain.Source, reason string) error {
	return classify(a.checks.SaveUnavailable(ctx, id, src, reason))
}
