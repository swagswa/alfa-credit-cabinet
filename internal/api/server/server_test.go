package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

func TestServer(t *testing.T) {
	tokens := token.New("test-secret", time.Hour)
	enf, err := rbac.New()
	require.NoError(t, err)

	newServer := func(dbErr error) http.Handler {
		health := usecase.NewHealth(map[string]usecase.Check{
			"db":       func(context.Context) error { return dbErr },
			"temporal": func(context.Context) error { return nil },
		})
		e, newErr := server.New(server.Deps{
			Log:     zap.NewNop(),
			Handler: handler.New(health),
			Tokens:  tokens,
			RBAC:    enf,
		})
		require.NoError(t, newErr)
		return e
	}
	client, err := tokens.Issue("u1", "client")
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
		{name: "разрешено, но ещё не сделано", method: "GET", path: "/api/me", session: client, status: 501},
		{name: "роли не положено", method: "GET", path: "/api/applications", session: client, status: 403},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, http.NoBody)
			if tt.session != "" {
				req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: tt.session})
			}
			rec := httptest.NewRecorder()

			newServer(tt.dbErr).ServeHTTP(rec, req)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status >= 400 && tt.status != 503 {
				require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			}
		})
	}
}
