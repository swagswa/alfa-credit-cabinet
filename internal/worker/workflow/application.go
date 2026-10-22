package workflow

import (
	"fmt"

	"github.com/google/uuid"
	"go.temporal.io/sdk/workflow"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

var system = domain.Actor{Role: domain.RoleSystem}

// ApplicationWorkflow ведёт заявку от подачи до выдачи, id workflow равен id заявки.
// Стартует подача, заявка к этому моменту уже в submitted.
func ApplicationWorkflow(ctx workflow.Context, id uuid.UUID) error {
	var a *activity.ApplicationActivities

	ctx = WithDBActivityOptions(ctx)

	err := workflow.ExecuteActivity(ctx, a.Transition, id, domain.Move{To: domain.StatusChecking, Actor: system}).
		Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("start checking %s: %w", id, err)
	}

	return nil
}
