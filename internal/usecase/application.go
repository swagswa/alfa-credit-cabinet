package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

type ApplicationUseCase interface {
	// Transition единственное место, где меняется статус заявки. Повтор того же
	// перехода ничего не делает, поэтому его можно звать из activity с ретраями.
	Transition(ctx context.Context, id uuid.UUID, m domain.Move) error
}

type applicationUseCase struct {
	db   *postgres.Client
	apps repo.ApplicationRepository
}

func NewApplication(db *postgres.Client, apps repo.ApplicationRepository) ApplicationUseCase {
	return &applicationUseCase{db: db, apps: apps}
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
