package usecase

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

type StatsUseCase interface {
	// Stats сводка за последние days дней. Кому её видно, решает casbin.
	Stats(ctx context.Context, days int) (domain.Stats, error)
}

type statsUseCase struct {
	db    *postgres.Client
	stats repo.StatsRepository
}

func NewStats(db *postgres.Client, stats repo.StatsRepository) StatsUseCase {
	return &statsUseCase{db: db, stats: stats}
}

// сводка и разбивка по статусам из одного снимка: иначе подача между двумя запросами
// разводит «подано» и сумму по статусам
func (u *statsUseCase) Stats(ctx context.Context, days int) (domain.Stats, error) {
	var s domain.Stats
	err := u.db.Snapshot(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		s, err = u.stats.WithTx(tx).Stats(ctx, time.Now().AddDate(0, 0, -days))
		return err
	})
	s.Days = days
	return s, err
}
