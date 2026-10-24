package repo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type StatsRepository interface {
	// Stats считает сводку по заявкам, поданным начиная с since. Days не заполняет.
	Stats(ctx context.Context, since time.Time) (domain.Stats, error)
	WithTx(tx pgx.Tx) StatsRepository
}

type statsRepo struct {
	q *db.Queries
}

func NewStats(q *db.Queries) StatsRepository {
	return &statsRepo{q: q}
}

func (r *statsRepo) WithTx(tx pgx.Tx) StatsRepository {
	return &statsRepo{q: r.q.WithTx(tx)}
}

func (r *statsRepo) Stats(ctx context.Context, since time.Time) (domain.Stats, error) {
	sum, err := r.q.StatsSummary(ctx, since)
	if err != nil {
		return domain.Stats{}, err
	}
	rows, err := r.q.StatsByStatus(ctx, since)
	if err != nil {
		return domain.Stats{}, err
	}
	s := domain.Stats{
		Submitted:      int(sum.Submitted),
		Decided:        int(sum.Decided),
		AutoDecided:    int(sum.AutoDecided),
		Approved:       int(sum.Approved),
		DecisionMedian: time.Duration(sum.MedianSec) * time.Second,
		DecisionP90:    time.Duration(sum.P90Sec) * time.Second,
		IssuedKop:      sum.IssuedKop,
		ByStatus:       make([]domain.StatusCount, len(rows)),
	}
	for i, row := range rows {
		s.ByStatus[i] = domain.StatusCount{
			Status: domain.Status(row.Status), Count: int(row.Count), Overdue: int(row.Overdue),
		}
	}
	return s, nil
}
