package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestSeedDemo(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	seed := postgres.Exec(migrations.SeedDemo)

	// второй прогон это повторный старт api, дублей быть не должно
	for range 2 {
		require.NoError(t, seed(t.Context(), pool))
	}

	count := func(sql string) int {
		var n int
		require.NoError(t, pool.QueryRow(t.Context(), sql).Scan(&n))
		return n
	}
	require.Equal(t, 8, count("SELECT count(*) FROM users"))
	require.Equal(t, 5, count("SELECT count(*) FROM companies"))
	require.Equal(t, 5, count("SELECT count(*) FROM company_members"))
	require.Equal(t, 1, count("SELECT count(*) FROM users WHERE role = 'underwriter'"))

	// у каждого клиента ровно одна компания, у сотрудников ни одной
	require.Equal(t, 0, count(`SELECT count(*) FROM users u
		WHERE (u.role = 'client') <> EXISTS (SELECT 1 FROM company_members m WHERE m.user_id = u.id)`))
	// профиль показывает контакты клиента и расшифровку ОКВЭД
	require.Equal(t, 0, count("SELECT count(*) FROM users WHERE role = 'client' AND (phone = '' OR email = '')"))
	require.Equal(t, 0, count("SELECT count(*) FROM companies WHERE okved_name = ''"))
}
