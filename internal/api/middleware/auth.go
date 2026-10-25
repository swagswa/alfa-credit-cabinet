// Package middleware: сессия из cookie и права ролей на эндпоинты.
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

const SessionCookie = "session"

type actorKey struct{}

type ipKey struct{}

// ActorFrom достаёт сессию в хендлере. Strict-хендлеры получают
// context.Context, а не echo.Context, поэтому всё нужное лежит в контексте запроса.
func ActorFrom(ctx context.Context) (domain.Session, bool) {
	s, ok := ctx.Value(actorKey{}).(domain.Session)
	return s, ok
}

// ClientIP отдаёт адрес клиента с учётом X-Forwarded-For от nginx.
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(ipKey{}).(string)
	return ip
}

// Auth находит сессию по cookie и проверяет право роли на путь и метод.
// Запрос без действующей сессии идёт с ролью anonymous: публичные операции
// описаны в той же политике.
func Auth(auth usecase.AuthUseCase, enf *rbac.Enforcer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// неизвестный путь попадает в групповой catch-all echo (/api/*):
			// пусть отвечает 404, а не 401/403 от политики
			if p := c.Path(); p == "" || strings.HasSuffix(p, "/*") {
				return next(c)
			}
			req := c.Request()
			s, err := session(c, auth)
			if err != nil {
				return err
			}
			if err := allow(enf, s, req); err != nil {
				return err
			}

			ctx := context.WithValue(req.Context(), ipKey{}, c.RealIP())
			if s.ID != "" {
				ctx = context.WithValue(ctx, actorKey{}, s)
			}
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

func allow(enf *rbac.Enforcer, s domain.Session, req *http.Request) error {
	role := string(s.Role)
	if role == "" {
		role = rbac.Anonymous
	}
	ok, err := enf.Allowed(role, req.URL.Path, req.Method)
	switch {
	case err != nil:
		return fmt.Errorf("rbac: %w", err)
	case ok:
		return nil
	case role == rbac.Anonymous:
		return echo.NewHTTPError(http.StatusUnauthorized)
	default:
		return echo.NewHTTPError(http.StatusForbidden)
	}
}

// session: нет cookie, битая подпись, истёкший срок или отозванная сессия
// это одно и то же, аноним. Недоступный Redis так не маскируем, это 500.
func session(c echo.Context, auth usecase.AuthUseCase) (domain.Session, error) {
	ck, err := c.Cookie(SessionCookie)
	if errors.Is(err, http.ErrNoCookie) {
		return domain.Session{}, nil
	}
	s, err := auth.Resolve(c.Request().Context(), ck.Value)
	if errors.Is(err, domain.ErrUnauthorized) {
		return domain.Session{}, nil
	}
	return s, err
}
