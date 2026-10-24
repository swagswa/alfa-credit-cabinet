package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	txAttempts   = 5
	txRetryDelay = 10 * time.Millisecond
)

// TxFunc может выполниться несколько раз, поэтому внутри только работа с tx,
// без внешних побочных эффектов.
type TxFunc func(ctx context.Context, tx pgx.Tx) error

// WithTx выполняет fn в транзакции. На serialization failure и deadlock
// повторяет её целиком, остальные ошибки возвращает сразу.
func (c *Client) WithTx(ctx context.Context, fn TxFunc) error {
	var err error
	for attempt := 1; attempt <= txAttempts; attempt++ {
		err = pgx.BeginFunc(ctx, c.Pool, func(tx pgx.Tx) error { return fn(ctx, tx) })
		if err == nil {
			return nil
		}
		if !retryable(err) {
			return fmt.Errorf("tx: %w", err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("tx retry: %w", ctx.Err())
		case <-time.After(time.Duration(attempt) * txRetryDelay):
		}
	}
	return fmt.Errorf("tx failed after %d attempts: %w", txAttempts, err)
}

// Snapshot читает в одном снимке базы: несколько запросов видят одно и то же состояние,
// даже если между ними кто-то закоммитил
func (c *Client) Snapshot(ctx context.Context, fn TxFunc) error {
	opts := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if err := pgx.BeginTxFunc(ctx, c.Pool, opts, func(tx pgx.Tx) error { return fn(ctx, tx) }); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	return nil
}

func retryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	// 40001 serialization_failure, 40P01 deadlock_detected
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}
