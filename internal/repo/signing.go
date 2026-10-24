package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// SigningRepository коды подписи и смоделированная выдача.
type SigningRepository interface {
	WithTx(tx pgx.Tx) SigningRepository
	// SaveCode ставит новый код вместо старого, если он был
	SaveCode(ctx context.Context, appID uuid.UUID, c domain.SigningCode) error
	// LockCode берёт код FOR UPDATE, кода нет: ErrNotFound
	LockCode(ctx context.Context, appID uuid.UUID) (domain.SigningCode, error)
	SetAttempts(ctx context.Context, appID uuid.UUID, n int) error
	DeleteCode(ctx context.Context, appID uuid.UUID) error
	// Payout пишет выдачу один раз, повтор ничего не меняет
	Payout(ctx context.Context, appID uuid.UUID, amountKop int64) error
}

type signingRepo struct {
	q *db.Queries
}

func NewSigning(conn db.DBTX) SigningRepository {
	return &signingRepo{q: db.New(conn)}
}

func (r *signingRepo) WithTx(tx pgx.Tx) SigningRepository {
	return &signingRepo{q: r.q.WithTx(tx)}
}

func (r *signingRepo) SaveCode(ctx context.Context, appID uuid.UUID, c domain.SigningCode) error {
	err := r.q.SaveSigningCode(ctx, db.SaveSigningCodeParams{
		ApplicationID: appID,
		CodeHash:      c.Hash,
		AttemptsLeft:  int16(c.AttemptsLeft),
		ExpiresAt:     c.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("save signing code %s: %w", appID, err)
	}
	return nil
}

func (r *signingRepo) LockCode(ctx context.Context, appID uuid.UUID) (domain.SigningCode, error) {
	row, err := r.q.LockSigningCode(ctx, appID)
	if err != nil {
		return domain.SigningCode{}, notFound(err, "signing code")
	}
	return domain.SigningCode{Hash: row.CodeHash, AttemptsLeft: int(row.AttemptsLeft), ExpiresAt: row.ExpiresAt}, nil
}

func (r *signingRepo) SetAttempts(ctx context.Context, appID uuid.UUID, n int) error {
	err := r.q.SetSigningAttempts(ctx, db.SetSigningAttemptsParams{ApplicationID: appID, AttemptsLeft: int16(n)})
	if err != nil {
		return fmt.Errorf("signing attempts %s: %w", appID, err)
	}
	return nil
}

func (r *signingRepo) DeleteCode(ctx context.Context, appID uuid.UUID) error {
	if err := r.q.DeleteSigningCode(ctx, appID); err != nil {
		return fmt.Errorf("delete signing code %s: %w", appID, err)
	}
	return nil
}

func (r *signingRepo) Payout(ctx context.Context, appID uuid.UUID, amountKop int64) error {
	if err := r.q.AddPayout(ctx, db.AddPayoutParams{ApplicationID: appID, AmountKop: amountKop}); err != nil {
		return fmt.Errorf("payout %s: %w", appID, err)
	}
	return nil
}
