// Package usecase: бизнес-логика и транзакции. На область интерфейс и
// неэкспортируемая реализация в одном файле, конструктор возвращает интерфейс.
package usecase

import (
	"context"
	"time"
)

const checkTimeout = 2 * time.Second

// Check проверяет одну зависимость: Postgres, Temporal.
type Check func(ctx context.Context) error

type HealthUseCase interface {
	// Check отдаёт ошибку по каждой зависимости, nil значит отвечает.
	Check(ctx context.Context) map[string]error
}

type healthUseCase struct {
	checks map[string]Check
}

func NewHealth(checks map[string]Check) HealthUseCase {
	return &healthUseCase{checks: checks}
}

func (u *healthUseCase) Check(ctx context.Context) map[string]error {
	res := make(map[string]error, len(u.checks))
	for name, check := range u.checks {
		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		res[name] = check(checkCtx)
		cancel()
	}
	return res
}
