package workflow

import (
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

var system = domain.Actor{Role: domain.RoleSystem}

// ApplicationWorkflow ведёт заявку от подачи до выдачи, id workflow равен id заявки.
// Стартует подача, заявка к этому моменту уже в submitted.
func ApplicationWorkflow(ctx workflow.Context, id uuid.UUID) error {
	if err := transition(ctx, id, domain.Move{To: domain.StatusChecking, Actor: system}); err != nil {
		return err
	}

	statuses, err := fetchSources(ctx, id)
	if err != nil {
		return err
	}
	// стоп-факторы про сами источники; запрос сильнее ручного, как у остальных
	switch {
	case statuses[domain.SourceEGRUL] == domain.CheckNotFound:
		m := domain.Move{To: domain.StatusNeedInfo, Actor: system, Codes: []string{domain.CodeNotFound}}
		return transition(ctx, id, m)
	case slices.ContainsFunc(domain.Sources, func(s domain.Source) bool { return statuses[s] == domain.CheckUnavailable }):
		m := domain.Move{To: domain.StatusManualReview, Actor: system, Codes: []string{domain.CodeSourceUnavailable}}
		return transition(ctx, id, m)
	}

	return nil
}

// fetchSources опрашивает источники разом. Источник, который так и не ответил,
// записывается как unavailable, остальные дорабатывают.
func fetchSources(ctx workflow.Context, id uuid.UUID) (map[domain.Source]domain.CheckStatus, error) {
	var a *activity.CheckActivities

	srcCtx := WithSourceActivityOptions(ctx)
	futures := make([]workflow.Future, len(domain.Sources))
	for i, src := range domain.Sources {
		futures[i] = workflow.ExecuteActivity(srcCtx, a.FetchSource, id, src)
	}

	// ответы собираем в порядке domain.Sources, а не по готовности: так реплей детерминирован
	statuses := make(map[domain.Source]domain.CheckStatus, len(domain.Sources))
	for i, src := range domain.Sources {
		var status domain.CheckStatus
		err := futures[i].Get(ctx, &status)
		if err != nil {
			workflow.GetLogger(ctx).Warn("source unavailable", "source", src, "error", err)
			err = workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.SaveUnavailable, id, src, reason(err)).
				Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("save unavailable %s %s: %w", src, id, err)
			}
			status = domain.CheckUnavailable
		}
		statuses[src] = status
	}
	return statuses, nil
}

func transition(ctx workflow.Context, id uuid.UUID, m domain.Move) error {
	var a *activity.ApplicationActivities

	err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Transition, id, m).Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("transition %s to %s: %w", id, m.To, err)
	}
	return nil
}

// reason достаёт причину из обёртки temporal: в карточку нужна она, а не id событий истории.
func reason(err error) string {
	if appErr, ok := errors.AsType[*temporal.ApplicationError](err); ok {
		return appErr.Message()
	}
	return err.Error()
}
