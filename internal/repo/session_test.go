package repo_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestSessionRepository(t *testing.T) {
	rdb := testenv.Redis(t)
	r := repo.NewSession(rdb)
	ctx := t.Context()

	s := domain.Session{ID: "abc", UserID: uuid.New(), Role: domain.RoleUnderwriter}
	require.NoError(t, r.Save(ctx, s, time.Hour))

	got, err := r.Find(ctx, "abc")
	require.NoError(t, err)
	require.Equal(t, s, got)
	ttl := rdb.TTL(ctx, "session:abc").Val()
	require.Greater(t, ttl, 59*time.Minute, "сессия живёт столько же, сколько токен")

	require.NoError(t, r.Delete(ctx, "abc"))
	_, err = r.Find(ctx, "abc")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSessionHit(t *testing.T) {
	rdb := testenv.Redis(t)
	r := repo.NewSession(rdb)
	ctx := t.Context()

	for want := int64(1); want <= 3; want++ {
		n, err := r.Hit(ctx, "login_attempts:10.0.0.1", time.Minute)
		require.NoError(t, err)
		require.Equal(t, want, n)
	}
	ttl := rdb.TTL(ctx, "login_attempts:10.0.0.1").Val()
	require.Positive(t, ttl, "у счётчика есть срок, иначе адрес заблокирован навсегда")
	require.LessOrEqual(t, ttl, time.Minute)
}
