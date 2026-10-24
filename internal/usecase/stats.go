package usecase

import (
	"context"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

type StatsUseCase interface {
	// Stats сводка за последние days дней. Кому её видно, решает casbin.
	Stats(ctx context.Context, days int) (domain.Stats, error)
}

type statsUseCase struct {
	stats repo.StatsRepository
}

func NewStats(stats repo.StatsRepository) StatsUseCase {
	return &statsUseCase{stats: stats}
}

func (u *statsUseCase) Stats(ctx context.Context, days int) (domain.Stats, error) {
	s, err := u.stats.Stats(ctx, time.Now().AddDate(0, 0, -days))
	s.Days = days
	return s, err
}
