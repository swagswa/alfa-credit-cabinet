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

func TestCheckRepository(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
		SELECT company_id, user_id, 'k', 'working_capital', 'purchase', 100000000, 12 FROM company_members LIMIT 1
		RETURNING id`).Scan(&id)
	require.NoError(t, err)

	r := repo.NewCheck(db.New(pool))
	require.NoError(t, r.Add(ctx, id, domain.CheckResult{
		Source: domain.SourceFNS, Status: domain.CheckOK, Payload: []byte(`{"msp_category": "micro"}`),
	}))
	require.NoError(t, r.Add(ctx, id, domain.CheckResult{Source: domain.SourceBKI, Status: domain.CheckNotFound}))
	require.NoError(t, r.Add(ctx, id, domain.CheckResult{
		Source: domain.SourceEGRUL, Status: domain.CheckUnavailable, Error: "egrul: 503",
	}))

	// ok без ответа схема не пропустит
	err = r.Add(ctx, id, domain.CheckResult{Source: domain.SourceInternal, Status: domain.CheckOK})
	require.Error(t, err)

	var got []string
	rows, err := pool.Query(ctx, `SELECT source || ' ' || status || ' ' || coalesce(payload->>'msp_category', '-')
		|| ' ' || coalesce(error, '-') FROM check_results WHERE application_id = $1 ORDER BY id`, id)
	require.NoError(t, err)
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		got = append(got, s)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"fns ok micro -", "bki not_found - -", "egrul unavailable - egrul: 503"}, got)
}
