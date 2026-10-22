package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

type fakeUsers struct {
	user domain.User
	hash []byte
}

func (f *fakeUsers) Credentials(_ context.Context, login string) (domain.User, []byte, error) {
	if login != f.user.Login {
		return domain.User{}, nil, domain.ErrNotFound
	}
	return f.user, f.hash, nil
}

func (f *fakeUsers) ByID(context.Context, uuid.UUID) (domain.User, error) { return f.user, nil }

func (f *fakeUsers) Memberships(context.Context, uuid.UUID) ([]domain.Membership, error) {
	return []domain.Membership{{INN: "0000100012", Role: "director"}}, nil
}

type fakeSessions struct {
	saved map[string]domain.Session
	hits  map[string]int64
}

func (f *fakeSessions) Save(_ context.Context, s domain.Session, _ time.Duration) error {
	f.saved[s.ID] = s
	return nil
}

func (f *fakeSessions) Find(_ context.Context, id string) (domain.Session, error) {
	s, ok := f.saved[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Delete(_ context.Context, id string) error {
	delete(f.saved, id)
	return nil
}

func (f *fakeSessions) Hit(_ context.Context, key string, _ time.Duration) (int64, error) {
	f.hits[key]++
	return f.hits[key], nil
}

func newAuth(t *testing.T) (usecase.AuthUseCase, *fakeSessions) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("demo12345"), bcrypt.MinCost)
	require.NoError(t, err)
	users := &fakeUsers{user: domain.User{ID: uuid.New(), Login: "coffee", Role: domain.RoleClient}, hash: hash}
	sessions := &fakeSessions{saved: map[string]domain.Session{}, hits: map[string]int64{}}
	limit := usecase.LoginLimit{Attempts: 3, Window: time.Minute}
	return usecase.NewAuth(users, sessions, token.New("secret", time.Hour), limit), sessions
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name, login, password string
		err                   error
	}{
		{"верный пароль", "coffee", "demo12345", nil},
		{"неверный пароль", "coffee", "wrong", domain.ErrUnauthorized},
		// тот же ответ, что и на неверный пароль: по ошибке не узнать, есть ли логин
		{"нет такого логина", "nobody", "demo12345", domain.ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, sessions := newAuth(t)

			me, raw, err := auth.Login(t.Context(), tt.login, tt.password, "10.0.0.1")

			require.ErrorIs(t, err, tt.err)
			if tt.err != nil {
				require.Empty(t, sessions.saved, "сессия не создаётся")
				return
			}
			require.Equal(t, "coffee", me.User.Login)
			require.Len(t, me.Memberships, 1)
			s, err := auth.Resolve(t.Context(), raw)
			require.NoError(t, err)
			require.Equal(t, domain.RoleClient, s.Role)
		})
	}
}

func TestLoginLimit(t *testing.T) {
	auth, _ := newAuth(t)
	for range 3 {
		_, _, err := auth.Login(t.Context(), "coffee", "wrong", "10.0.0.1")
		require.ErrorIs(t, err, domain.ErrUnauthorized)
	}

	_, _, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.ErrorIs(t, err, domain.ErrTooManyRequests, "лимит срабатывает раньше проверки пароля")

	_, _, err = auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.2")
	require.NoError(t, err, "другой адрес не задет")
}

func TestLogoutRevokes(t *testing.T) {
	auth, _ := newAuth(t)
	_, raw, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)
	s, err := auth.Resolve(t.Context(), raw)
	require.NoError(t, err)

	require.NoError(t, auth.Logout(t.Context(), s.ID))

	_, err = auth.Resolve(t.Context(), raw)
	require.ErrorIs(t, err, domain.ErrUnauthorized, "токен жив по подписи, но сессии больше нет")
}

func TestResolveGarbage(t *testing.T) {
	auth, _ := newAuth(t)
	_, err := auth.Resolve(t.Context(), "junk")
	require.ErrorIs(t, err, domain.ErrUnauthorized)
}
