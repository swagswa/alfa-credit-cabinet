package repo_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestStats(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))

	coffee := uuid.MustParse("01930000-0000-7000-8000-000000000001")
	coffeeCo := uuid.MustParse("01930000-0000-7000-8000-000000000101")
	uw := uuid.MustParse("01930000-0000-7000-8000-000000000012")
	now := time.Now()

	// apply заводит поданную заявку в статусе status; due в прошлом значит просрочку
	apply := func(status string, submitted time.Time, due *time.Time) uuid.UUID {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop,
				term_months, status, submitted_at, bki_consent_at, pd_consent_at, due_at,
				offer_product, offer_amount_kop, offer_term_months, offer_rate_bp)
			VALUES ($1, $2, gen_random_uuid()::text, 'working_capital', 'purchase', 100000000, 12,
				$3, $4, $4, $4, $5, 'working_capital', 100000000, 12, 1900)
			RETURNING id`, coffeeCo, coffee, status, submitted, due).Scan(&id)
		require.NoError(t, err)
		return id
	}
	decide := func(id uuid.UUID, to string, actor *uuid.UUID, at time.Time) {
		role := "system"
		if actor != nil {
			role = "underwriter"
		}
		_, err := pool.Exec(ctx, `INSERT INTO application_events
			(application_id, to_status, actor_id, actor_role, created_at) VALUES ($1, $2, $3, $4, $5)`,
			id, to, actor, role, at)
		require.NoError(t, err)
	}

	start := now.Add(-time.Hour)
	closed := apply("closed", start, nil)
	decide(closed, "approved", nil, start.Add(time.Minute))
	_, err := pool.Exec(ctx, `INSERT INTO payouts (application_id, amount_kop, account)
		VALUES ($1, 100000000, '40702')`, closed)
	require.NoError(t, err)

	counter := apply("counter_offer", start, nil)
	decide(counter, "manual_review", nil, start.Add(time.Second))
	decide(counter, "counter_offer", &uw, start.Add(10*time.Minute))
	// повторное решение после новых проверок время не портит
	decide(counter, "counter_offer", &uw, start.Add(50*time.Minute))

	rejected := apply("rejected", start, nil)
	decide(rejected, "rejected", nil, start.Add(20*time.Second))

	apply("need_info", start, new(now.Add(-time.Minute)))
	apply("closed", now.AddDate(0, 0, -40), nil) // до периода

	s, err := repo.NewStats(db.New(pool)).Stats(ctx, now.AddDate(0, 0, -30))
	require.NoError(t, err)
	require.Equal(t, domain.Stats{
		Submitted:   4,
		Decided:     3,
		AutoDecided: 2,
		Approved:    2,
		// 20 с, 1 мин, 10 мин: медиана посередине, p90 между второй и третьей
		DecisionMedian: time.Minute,
		DecisionP90:    492 * time.Second,
		IssuedKop:      100000000,
		ByStatus: []domain.StatusCount{
			{Status: domain.StatusClosed, Count: 1},
			{Status: domain.StatusCounterOffer, Count: 1},
			{Status: domain.StatusNeedInfo, Count: 1, Overdue: 1},
			{Status: domain.StatusRejected, Count: 1},
		},
	}, s)

	empty, err := repo.NewStats(db.New(pool)).Stats(ctx, now.Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, empty.Submitted)
	require.Zero(t, empty.DecisionMedian)
	require.Empty(t, empty.ByStatus)
}
