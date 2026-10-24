package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type FeedRepository interface {
	// Profile отдаёт профиль компании пользователя. companyID нулевой значит
	// первую по имени; чужая компания это ErrNotFound.
	Profile(ctx context.Context, userID, companyID uuid.UUID) (domain.Profile, error)
	// Events: смены статусов заявок компании, новые сверху.
	Events(ctx context.Context, userID, companyID uuid.UUID, limit int) ([]domain.StatusEvent, error)
}

type feedRepo struct {
	q *db.Queries
}

func NewFeed(q *db.Queries) FeedRepository {
	return &feedRepo{q: q}
}

func (r *feedRepo) Profile(ctx context.Context, userID, companyID uuid.UUID) (domain.Profile, error) {
	row, err := r.q.FeedCompany(ctx, db.FeedCompanyParams{
		UserID:    userID,
		CompanyID: pgtype.UUID{Bytes: companyID, Valid: companyID != uuid.Nil},
	})
	if err != nil {
		return domain.Profile{}, notFound(err, "feed company")
	}
	p := domain.Profile{CompanyID: row.ID, Okved: row.Okved, LastStatus: row.LastStatus}
	// живой запрос в банк ради ленты не делаем: она перечитывается на каждое
	// событие и съедала бы сбои, заданные моку для сценария проверок
	var b struct {
		Products []string `json:"products"`
		Turnover []struct {
			Credit int64 `json:"credit"`
		} `json:"turnover_12m"`
	}
	if row.Bank == nil {
		return p, nil
	}
	if err = json.Unmarshal(row.Bank, &b); err != nil {
		return p, fmt.Errorf("feed bank data %s: %w", row.ID, err)
	}
	p.Bank, p.Products = true, b.Products
	for _, m := range b.Turnover {
		p.TurnoverMonthRub += m.Credit
	}
	if n := len(b.Turnover); n > 0 {
		p.TurnoverMonthRub /= int64(n)
	}
	return p, nil
}

func (r *feedRepo) Events(ctx context.Context, userID, companyID uuid.UUID, limit int) ([]domain.StatusEvent, error) {
	rows, err := r.q.FeedEvents(ctx, db.FeedEventsParams{
		UserID:    userID,
		CompanyID: companyID,
		Lim:       int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("feed events %s: %w", companyID, err)
	}
	out := make([]domain.StatusEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.StatusEvent{
			ID:            row.ID,
			ApplicationID: row.ApplicationID,
			Number:        row.Number,
			Status:        domain.Status(row.Status),
			At:            row.CreatedAt,
		})
	}
	return out, nil
}
