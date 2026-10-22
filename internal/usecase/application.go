package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

// Очереди и имена процессов Temporal. Живут здесь, а не в worker/workflow:
// процессы стартует usecase, а workflow сам зовёт usecase через activity.
const (
	QueueCredit = "credit"
	QueueDocs   = "docs"

	ApplicationWorkflowName = "ApplicationWorkflow"
)

// Workflows стартует процессы, на стенде это temporalsvc.Client.
type Workflows interface {
	Start(ctx context.Context, queue, id, workflow string, args ...any) error
}

type ApplicationUseCase interface {
	// Transition единственное место, где меняется статус заявки. Повтор того же
	// перехода ничего не делает, поэтому его можно звать из activity с ретраями.
	Transition(ctx context.Context, id uuid.UUID, m domain.Move) error
	// StartProcess запускает ApplicationWorkflow заявки, id процесса равен id заявки.
	// Второй запуск для той же заявки даёт ErrConflict.
	StartProcess(ctx context.Context, id uuid.UUID) error
}

type applicationUseCase struct {
	db        *postgres.Client
	apps      repo.ApplicationRepository
	workflows Workflows
}

func NewApplication(db *postgres.Client, apps repo.ApplicationRepository, workflows Workflows) ApplicationUseCase {
	return &applicationUseCase{db: db, apps: apps, workflows: workflows}
}

func (u *applicationUseCase) StartProcess(ctx context.Context, id uuid.UUID) error {
	err := u.workflows.Start(ctx, QueueCredit, id.String(), ApplicationWorkflowName, id)
	if errors.Is(err, temporalsvc.ErrAlreadyStarted) {
		return fmt.Errorf("%w: процесс заявки уже запущен", domain.ErrConflict)
	}
	return err
}

func (u *applicationUseCase) Transition(ctx context.Context, id uuid.UUID, m domain.Move) error {
	return u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		apps := u.apps.WithTx(tx)
		app, err := apps.Lock(ctx, id)
		if err != nil {
			return err
		}
		// идемпотентность раньше версии: повтор после коммита видит уже новую версию
		switch {
		case app.Status == m.To:
			return nil
		case m.Version != 0 && m.Version != app.Version:
			return fmt.Errorf("%w: заявка уже изменилась, обновите карточку", domain.ErrConflict)
		case !domain.CanTransition(app.Status, m.To, m.Actor.Role):
			return fmt.Errorf("%w: из %s в %s ролью %s", domain.ErrIllegalTransition, app.Status, m.To, m.Actor.Role)
		}
		return apps.Move(ctx, app, m)
	})
}
