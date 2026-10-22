package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type ApplicationRepository interface {
	WithTx(tx pgx.Tx) ApplicationRepository
	// Lock берёт строку заявки FOR UPDATE до конца транзакции.
	Lock(ctx context.Context, id uuid.UUID) (domain.Application, error)
	// Move пишет новый статус, событие аудита и уведомление для SSE.
	Move(ctx context.Context, app domain.Application, m domain.Move) error
}

type applicationRepo struct {
	q *db.Queries
}

func NewApplication(q *db.Queries) ApplicationRepository {
	return &applicationRepo{q: q}
}

func (r *applicationRepo) WithTx(tx pgx.Tx) ApplicationRepository {
	return &applicationRepo{q: r.q.WithTx(tx)}
}

func (r *applicationRepo) Lock(ctx context.Context, id uuid.UUID) (domain.Application, error) {
	row, err := r.q.LockApplication(ctx, id)
	if err != nil {
		return domain.Application{}, notFound(err, "lock application "+id.String())
	}
	return domain.Application{
		ID:        row.ID,
		CompanyID: row.CompanyID,
		Status:    domain.Status(row.Status),
		Version:   int(row.Version),
	}, nil
}

func (r *applicationRepo) Move(ctx context.Context, app domain.Application, m domain.Move) error {
	due := pgtype.Timestamptz{}
	if m.DueAt != nil {
		due = pgtype.Timestamptz{Time: *m.DueAt, Valid: true}
	}
	version, err := r.q.SetApplicationStatus(ctx, db.SetApplicationStatusParams{
		ID: app.ID, Status: string(m.To), DueAt: due,
	})
	if err != nil {
		return fmt.Errorf("set status %s: %w", app.ID, err)
	}
	err = r.q.AddTransitionEvent(ctx, db.AddTransitionEventParams{
		ApplicationID: app.ID,
		FromStatus:    pgtype.Text{String: string(app.Status), Valid: true},
		ToStatus:      pgtype.Text{String: string(m.To), Valid: true},
		ActorID:       pgtype.UUID{Bytes: m.Actor.ID, Valid: m.Actor.ID != uuid.Nil},
		ActorRole:     string(m.Actor.Role),
		ReasonCodes:   m.Codes,
		Comment:       pgtype.Text{String: m.Comment, Valid: m.Comment != ""},
	})
	if err != nil {
		return fmt.Errorf("transition event %s: %w", app.ID, err)
	}
	err = r.q.NotifyApplication(ctx, db.NotifyApplicationParams{
		ID: app.ID, CompanyID: app.CompanyID, Status: string(m.To), Version: version,
	})
	if err != nil {
		return fmt.Errorf("notify %s: %w", app.ID, err)
	}
	return nil
}
