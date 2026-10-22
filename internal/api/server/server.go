// Package server собирает echo: общие middleware, сессия и права, проверка
// запросов по спеке, сгенерированный роутинг и единый ответ об ошибке.
package server

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	oapimw "github.com/oapi-codegen/echo-middleware"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

const basePath = "/api"

type Deps struct {
	Log     *zap.Logger
	Handler *handler.Handler
	Auth    usecase.AuthUseCase
	RBAC    *rbac.Enforcer
	// CORSOrigin задаётся только для dev-сервера Vite
	CORSOrigin string
}

func New(d Deps) (*echo.Echo, error) {
	spec, err := oapi.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = errorHandler(d.Log)
	// перед api стоит nginx, адрес клиента берём из X-Forwarded-For, но верим
	// ему только от приватных сетей, иначе лимит попыток входа обходится заголовком
	e.IPExtractor = echo.ExtractIPFromXFFHeader()

	e.Use(echomw.Recover(), echomw.RequestID(), requestLogger(d.Log))
	if d.CORSOrigin != "" {
		e.Use(echomw.CORSWithConfig(echomw.CORSConfig{AllowOrigins: []string{d.CORSOrigin}, AllowCredentials: true}))
	}

	// Prefix у валидатора не берём: в echo-middleware 1.1.0 он валидирует клон
	// запроса, вычитывает тело клона, а хендлеру достаётся пустое (на любом POST
	// с телом 400 EOF). Путь /api сопоставляется через servers из самой спеки,
	// сервер там относительный, хост не проверяется.
	validate := oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
		SilenceServersWarning: true,
		// исключённую из генерации операцию генератор вырезает и из встроенной
		// спеки, валидатор отвечал бы на SSE 405. Параметр проверяет сам хендлер
		Skipper: func(c echo.Context) bool { return c.Path() == basePath+"/events" },
		// права проверяет Auth, валидатору остаются только схемы
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	})
	// валидатор читает тело целиком в память, поэтому лимит раньше него: файл до 10 МБ
	// плюс multipart-обвязка, на большее сразу 413
	api := e.Group(basePath, middleware.Auth(d.Auth, d.RBAC), echomw.BodyLimit("11M"), validate)
	oapi.RegisterHandlers(api, oapi.NewStrictHandler(d.Handler, nil))
	api.GET("/events", d.Handler.Events)

	return e, nil
}
