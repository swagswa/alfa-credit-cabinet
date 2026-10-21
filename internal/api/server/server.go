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
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

const basePath = "/api"

type Deps struct {
	Log     *zap.Logger
	Handler *handler.Handler
	Tokens  *token.Issuer
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

	e.Use(echomw.Recover(), echomw.RequestID(), requestLogger(d.Log))
	if d.CORSOrigin != "" {
		e.Use(echomw.CORSWithConfig(echomw.CORSConfig{AllowOrigins: []string{d.CORSOrigin}, AllowCredentials: true}))
	}

	validate := oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
		Prefix:               basePath,
		DoNotValidateServers: true,
		// права проверяет Auth, валидатору остаются только схемы
		Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	})
	api := e.Group(basePath, middleware.Auth(d.Tokens, d.RBAC), validate)
	oapi.RegisterHandlers(api, oapi.NewStrictHandler(d.Handler, nil))
	api.GET("/events", d.Handler.Events)

	return e, nil
}
