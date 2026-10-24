package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type CheckRepository interface {
	// Add дописывает ответ источника. После need_info источники опрашиваются заново,
	// старые строки остаются для аудита.
	Add(ctx context.Context, id uuid.UUID, r domain.CheckResult) error
	// Failed сообщает в SSE о неудачной попытке. В базу она не пишется: последняя
	// строка по источнику это результат, по которому решают.
	Failed(ctx context.Context, id uuid.UUID, r domain.CheckResult) error
}

type checkRepo struct {
	q *db.Queries
}

func NewCheck(q *db.Queries) CheckRepository {
	return &checkRepo{q: q}
}

func (r *checkRepo) Add(ctx context.Context, id uuid.UUID, c domain.CheckResult) error {
	err := r.q.AddCheckResult(ctx, db.AddCheckResultParams{
		ApplicationID: id,
		Source:        string(c.Source),
		Status:        string(c.Status),
		Payload:       c.Payload,
		Error:         pgtype.Text{String: c.Error, Valid: c.Error != ""},
		Attempt:       int32(c.Attempt.N),
		Attempts:      int32(c.Attempt.Of),
	})
	if err != nil {
		return fmt.Errorf("check %s %s: %w", id, c.Source, err)
	}
	return nil
}

func (r *checkRepo) Failed(ctx context.Context, id uuid.UUID, c domain.CheckResult) error {
	err := r.q.NotifyCheckAttempt(ctx, db.NotifyCheckAttemptParams{
		ApplicationID: id,
		Source:        string(c.Source),
		Attempt:       int32(c.Attempt.N),
		Attempts:      int32(c.Attempt.Of),
		Error:         c.Error,
	})
	if err != nil {
		return fmt.Errorf("check attempt %s %s: %w", id, c.Source, err)
	}
	return nil
}
