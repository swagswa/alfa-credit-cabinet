package usecase

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

// Notifier пишет уведомления клиенту в той же транзакции, что и событие, о котором они:
// откатился переход, пропало и уведомление.
type Notifier struct {
	repo  repo.NotificationRepository
	rules domain.NoticeRules
}

func NewNotifier(n repo.NotificationRepository, rules domain.NoticeRules) *Notifier {
	return &Notifier{repo: n, rules: rules}
}

// Status: на статус без фразы уведомления нет.
func (n *Notifier) Status(ctx context.Context, tx pgx.Tx, id uuid.UUID, to domain.Status) error {
	ph, ok := n.rules.Statuses[to]
	if !ok {
		return nil
	}
	return n.send(ctx, tx, id, string(to), ph)
}

func (n *Notifier) SigningCode(ctx context.Context, tx pgx.Tx, id uuid.UUID, code string) error {
	return n.send(ctx, tx, id, domain.NoticeSigningCode, n.rules.SigningCode, "{code}", code)
}

func (n *Notifier) send(
	ctx context.Context, tx pgx.Tx, id uuid.UUID, kind string, ph domain.Phrase, extra ...string,
) error {
	r := n.repo.WithTx(tx)
	v, err := r.Vars(ctx, id)
	if err != nil {
		return err
	}
	vars := append([]string{
		"{number}", strconv.FormatInt(v.Number, 10),
		"{amount}", rubles(v.AmountKop) + " ₽",
		"{account}", domain.MaskAccount(v.Account),
	}, extra...)
	return r.Notify(ctx, id, kind, ph.Fill(vars...))
}

type NotificationUseCase interface {
	List(ctx context.Context, userID uuid.UUID, unread bool, limit int) ([]domain.Notification, error)
	// Read: чужое уведомление как несуществующее, ErrNotFound.
	Read(ctx context.Context, userID uuid.UUID, id int64) error
	ReadAll(ctx context.Context, userID uuid.UUID) error
}

// чтение ленты идёт прямо в репозиторий: своё от чужого отличает SQL
func NewNotification(n repo.NotificationRepository) NotificationUseCase {
	return n
}
