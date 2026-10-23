package repo_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestFeedOwnEventsOnly(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))

	// пользователи и компании из сида: кофейня и логистика
	coffee := uuid.MustParse("01930000-0000-7000-8000-000000000001")
	coffeeCo := uuid.MustParse("01930000-0000-7000-8000-000000000101")
	logistics := uuid.MustParse("01930000-0000-7000-8000-000000000002")
	logisticsCo := uuid.MustParse("01930000-0000-7000-8000-000000000102")
	apply := func(user, company uuid.UUID, statuses ...string) uuid.UUID {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
			VALUES ($1, $2, gen_random_uuid()::text, 'working_capital', 'purchase', 100000000, 12) RETURNING id`,
			company, user).Scan(&id)
		require.NoError(t, err)
		for _, s := range statuses {
			_, err = pool.Exec(ctx, `INSERT INTO application_events (application_id, to_status, actor_role)
				VALUES ($1, $2, 'system')`, id, s)
			require.NoError(t, err)
		}
		return id
	}
	own := apply(coffee, coffeeCo, "submitted", "checking")
	apply(logistics, logisticsCo, "submitted")
	_, err := pool.Exec(ctx, `INSERT INTO application_events (application_id, kind, actor_role)
		VALUES ($1, 'escalation', 'system')`, own)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO check_results (application_id, source, status, payload) VALUES
		($1, 'internal', 'ok', '{"products": ["account", "acquiring"], "turnover_12m": [{"credit": 100}, {"credit": 300}]}')`,
		own)
	require.NoError(t, err)

	r := repo.NewFeed(db.New(pool))

	events, err := r.Events(ctx, coffee, coffeeCo, 10)
	require.NoError(t, err)
	require.Len(t, events, 2, "эскалация и чужая заявка в ленту не попадают")
	require.Equal(t, domain.StatusChecking, events[0].Status, "новые сверху")
	for _, ev := range events {
		require.Equal(t, own, ev.ApplicationID)
	}

	// чужую компанию не отдают ни профилем, ни событиями
	_, err = r.Profile(ctx, coffee, logisticsCo)
	require.ErrorIs(t, err, domain.ErrNotFound)
	events, err = r.Events(ctx, coffee, logisticsCo, 10)
	require.NoError(t, err)
	require.Empty(t, events)

	p, err := r.Profile(ctx, coffee, uuid.Nil)
	require.NoError(t, err)
	require.Equal(t, domain.Profile{
		CompanyID: coffeeCo, Okved: "56.30", LastStatus: "draft",
		Bank: true, Products: []string{"account", "acquiring"}, TurnoverMonthRub: 200,
	}, p)

	// у менеджера компаний нет
	_, err = r.Profile(ctx, uuid.MustParse("01930000-0000-7000-8000-000000000011"), uuid.Nil)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
