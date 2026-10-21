// Package middleware: сессия из cookie и права ролей на эндпоинты.
package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

const SessionCookie = "session"

type Actor struct {
	UserID string
	Role   string
}

type actorKey struct{}

// ActorFrom достаёт пользователя в хендлере. Strict-хендлеры получают
// context.Context, а не echo.Context, поэтому пользователь лежит в контексте запроса.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Auth разбирает cookie сессии и проверяет право роли на путь и метод.
// Запрос без сессии идёт с ролью anonymous: публичные операции описаны в той же политике.
func Auth(tokens *token.Issuer, enf *rbac.Enforcer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			actor := session(c, tokens)
			req := c.Request()
			ok, err := enf.Allowed(actor.Role, req.URL.Path, req.Method)
			if err != nil {
				return fmt.Errorf("rbac: %w", err)
			}
			if !ok {
				if actor.Role == rbac.Anonymous {
					return echo.NewHTTPError(http.StatusUnauthorized)
				}
				return echo.NewHTTPError(http.StatusForbidden)
			}

			c.SetRequest(req.WithContext(context.WithValue(req.Context(), actorKey{}, actor)))
			return next(c)
		}
	}
}

// session: битая или истёкшая сессия это то же, что её отсутствие
func session(c echo.Context, tokens *token.Issuer) Actor {
	ck, err := c.Cookie(SessionCookie)
	if err != nil {
		return Actor{Role: rbac.Anonymous}
	}
	claims, err := tokens.Parse(ck.Value)
	if err != nil {
		return Actor{Role: rbac.Anonymous}
	}
	return Actor{UserID: claims.Subject, Role: claims.Role}
}
