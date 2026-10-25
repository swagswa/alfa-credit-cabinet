package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type NotificationRepository interface {
	WithTx(tx pgx.Tx) NotificationRepository
	// Notify пишет фразу каждому клиенту компании заявки.
	Notify(ctx context.Context, appID uuid.UUID, kind string, p domain.Phrase) error
	// Vars: номер заявки, сумма и счёт выдачи для подстановки в фразу.
	Vars(ctx context.Context, appID uuid.UUID) (domain.NoticeVars, error)
	// List: новые сверху.
	List(ctx context.Context, userID uuid.UUID, unread bool, limit int) ([]domain.Notification, error)
	// Read отмечает своё уведомление, чужое или несуществующее ErrNotFound.
	Read(ctx context.Context, userID uuid.UUID, id int64) error
	ReadAll(ctx context.Context, userID uuid.UUID) error
}

type notificationRepo struct {
	q *db.Queries
}

func NewNotification(conn db.DBTX) NotificationRepository {
	return &notificationRepo{q: db.New(conn)}
}

func (r *notificationRepo) WithTx(tx pgx.Tx) NotificationRepository {
	return &notificationRepo{q: r.q.WithTx(tx)}
}

func (r *notificationRepo) Notify(ctx context.Context, appID uuid.UUID, kind string, p domain.Phrase) error {
	err := r.q.AddNotifications(ctx, db.AddNotificationsParams{
		ApplicationID: appID, Kind: kind, Title: p.Title, Body: p.Text,
	})
	if err != nil {
		return fmt.Errorf("notify %s %s: %w", appID, kind, err)
	}
	return nil
}

func (r *notificationRepo) Vars(ctx context.Context, appID uuid.UUID) (domain.NoticeVars, error) {
	row, err := r.q.NoticeVars(ctx, appID)
	if err != nil {
		return domain.NoticeVars{}, notFound(err, "notice vars "+appID.String())
	}
	return domain.NoticeVars{Number: row.Number, AmountKop: row.AmountKop, Account: row.Account}, nil
}

func (r *notificationRepo) List(
	ctx context.Context, userID uuid.UUID, unread bool, limit int,
) ([]domain.Notification, error) {
	rows, err := r.q.ListNotifications(ctx, db.ListNotificationsParams{
		UserID: userID, Unread: unread, Lim: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("notifications %s: %w", userID, err)
	}
	out := make([]domain.Notification, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		n := domain.Notification{
			ID:            row.ID,
			ApplicationID: row.ApplicationID.Bytes,
			Kind:          row.Kind,
			Title:         row.Title,
			Text:          row.Body,
			CreatedAt:     row.CreatedAt,
		}
		if row.ReadAt.Valid {
			n.ReadAt = &row.ReadAt.Time
		}
		out = append(out, n)
	}
	return out, nil
}

func (r *notificationRepo) Read(ctx context.Context, userID uuid.UUID, id int64) error {
	n, err := r.q.ReadNotification(ctx, db.ReadNotificationParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("read notification %d: %w", id, err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *notificationRepo) ReadAll(ctx context.Context, userID uuid.UUID) error {
	if err := r.q.ReadAllNotifications(ctx, userID); err != nil {
		return fmt.Errorf("read all notifications %s: %w", userID, err)
	}
	return nil
}
