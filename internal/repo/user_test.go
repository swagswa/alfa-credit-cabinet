package repo_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestUserRepository(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(t.Context(), pool))
	r := repo.NewUser(db.New(pool))
	ctx := t.Context()

	u, hash, err := r.Credentials(ctx, "coffee")
	require.NoError(t, err)
	require.Equal(t, domain.RoleClient, u.Role)
	require.NoError(t, bcrypt.CompareHashAndPassword(hash, []byte("demo12345")))

	_, _, err = r.Credentials(ctx, "nobody")
	require.ErrorIs(t, err, domain.ErrNotFound)

	ms, err := r.Memberships(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	require.Equal(t, "0000100012", ms[0].INN)
	require.Equal(t, domain.RepresentativeRole("director"), ms[0].Role)

	staff, _, err := r.Credentials(ctx, "underwriter")
	require.NoError(t, err)
	ms, err = r.Memberships(ctx, staff.ID)
	require.NoError(t, err)
	require.Empty(t, ms, "у сотрудника компаний нет")

	_, err = r.ByID(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}
