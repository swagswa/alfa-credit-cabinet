package repo_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestEscalate(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
		SELECT company_id, user_id, 'k', 'working_capital', 'purchase', 100000000, 12 FROM company_members LIMIT 1
		RETURNING id`).Scan(&id)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(ctx, "LISTEN applications")
	require.NoError(t, err)

	require.NoError(t, repo.NewApplication(db.New(pool)).Escalate(ctx, id))

	var kind, role, status string
	var version int
	err = pool.QueryRow(ctx, `SELECT e.kind, e.actor_role, a.status, a.version
		FROM application_events e JOIN applications a ON a.id = e.application_id WHERE a.id = $1`, id).
		Scan(&kind, &role, &status, &version)
	require.NoError(t, err)
	require.Equal(t, []any{"escalation", "system", "draft", 1}, []any{kind, role, status, version})

	n, err := conn.Conn().WaitForNotification(ctx)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(n.Payload), &got))
	require.Equal(t, id.String(), got["application_id"])
	require.Equal(t, "draft", got["status"])
}
