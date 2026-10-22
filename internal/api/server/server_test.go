package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

const (
	recommend        = "/api/products/recommendation?goal=cash_gap&amount_kop=100000000"
	recommendLeasing = "/api/products/recommendation?goal=leasing&amount_kop=100000000&term_months=2"
)

type egrulDown struct{}

// одна учётка клиента: как в сиде, coffee с паролем demo12345
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
	return nil, nil
}

type fakeSessions struct {
	mu    sync.Mutex
	saved map[string]domain.Session
	hits  map[string]int64
}

func (f *fakeSessions) Save(_ context.Context, s domain.Session, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved[s.ID] = s
	return nil
}

func (f *fakeSessions) Find(_ context.Context, id string) (domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.saved[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.saved, id)
	return nil
}

func (f *fakeSessions) Hit(_ context.Context, key string, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[key]++
	return f.hits[key], nil
}

func newAuth(t *testing.T) usecase.AuthUseCase {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("demo12345"), bcrypt.MinCost)
	require.NoError(t, err)
	users := &fakeUsers{user: domain.User{ID: uuid.New(), Login: "coffee", Role: domain.RoleClient}, hash: hash}
	sessions := &fakeSessions{saved: map[string]domain.Session{}, hits: map[string]int64{}}
	limit := usecase.LoginLimit{Attempts: 3, Window: time.Minute}
	return usecase.NewAuth(users, sessions, token.New("test-secret", time.Hour), limit)
}

func newServer(t *testing.T, auth usecase.AuthUseCase, dbErr error) http.Handler {
	t.Helper()
	enf, err := rbac.New()
	require.NoError(t, err)
	catalog, err := rules.Products("../../../rules/products.yaml")
	require.NoError(t, err)
	health := usecase.NewHealth(map[string]usecase.Check{
		"db":       func(context.Context) error { return dbErr },
		"temporal": func(context.Context) error { return nil },
	})
	e, err := server.New(server.Deps{
		Log: zap.NewNop(),
		Handler: handler.New(handler.Deps{
			Health:     health,
			Products:   usecase.NewProduct(catalog),
			Companies:  usecase.NewCompany(egrulDown{}),
			Auth:       auth,
			SessionTTL: time.Hour,
		}),
		Auth: auth,
		RBAC: enf,
	})
	require.NoError(t, err)
	return e
}

func call(t *testing.T, h http.Handler, method, path, body, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: session})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (egrulDown) Company(context.Context, string) (domain.Company, error) {
	return domain.Company{}, domain.ErrUnavailable
}

func TestServer(t *testing.T) {
	auth := newAuth(t)
	_, client, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)

	tests := []struct {
		name    string
		dbErr   error
		method  string
		path    string
		session string
		status  int
	}{
		{name: "health без сессии", method: "GET", path: "/api/health", status: 200},
		{name: "health при упавшей базе", dbErr: errors.New("down"), method: "GET", path: "/api/health", status: 503},
		{name: "закрытая операция без сессии", method: "GET", path: "/api/me", status: 401},
		{name: "битая сессия как её отсутствие", method: "GET", path: "/api/me", session: "junk", status: 401},
		{name: "текущий пользователь", method: "GET", path: "/api/me", session: client, status: 200},
		{name: "роли не положено", method: "GET", path: "/api/applications", session: client, status: 403},
		{name: "продукты", method: "GET", path: "/api/products", session: client, status: 200},
		{name: "подбор", method: "GET", path: recommend + "&term_months=2", session: client, status: 200},
		{name: "подбор без срока", method: "GET", path: recommend, session: client, status: 400},
		{name: "подбор с чужой целью", method: "GET", path: recommendLeasing, session: client, status: 400},
		{name: "инн короче 10", method: "GET", path: "/api/companies/000010001", session: client, status: 400},
		{name: "инн с неверной контрольной", method: "GET", path: "/api/companies/0000100013", session: client,
			status: 400},
		{name: "источник лежит", method: "GET", path: "/api/companies/0000500010", session: client, status: 503},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(t, newServer(t, auth, tt.dbErr), tt.method, tt.path, "", tt.session)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status >= 400 && tt.status != 503 {
				require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestLoginLogout(t *testing.T) {
	h := newServer(t, newAuth(t), nil)

	rec := call(t, h, "POST", "/api/login", `{"login":"coffee","password":"wrong"}`, "")
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	require.Empty(t, rec.Result().Cookies())

	rec = call(t, h, "POST", "/api/login", `{"login":"coffee","password":"demo12345"}`, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	ck := rec.Result().Cookies()
	require.Len(t, ck, 1)
	require.True(t, ck[0].HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, ck[0].SameSite)
	session := ck[0].Value

	require.Equal(t, http.StatusOK, call(t, h, "GET", "/api/me", "", session).Code)

	rec = call(t, h, "POST", "/api/logout", "", session)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Negative(t, rec.Result().Cookies()[0].MaxAge, "cookie стирается")

	// токен по подписи ещё жив, но сессии на сервере больше нет
	require.Equal(t, http.StatusUnauthorized, call(t, h, "GET", "/api/me", "", session).Code)
}

func TestLoginLimit(t *testing.T) {
	h := newServer(t, newAuth(t), nil)
	for range 3 {
		require.Equal(
			t,
			http.StatusUnauthorized,
			call(t, h, "POST", "/api/login", `{"login":"coffee","password":"x"}`, "").Code,
		)
	}
	rec := call(t, h, "POST", "/api/login", `{"login":"coffee","password":"demo12345"}`, "")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "too_many_requests")
}
