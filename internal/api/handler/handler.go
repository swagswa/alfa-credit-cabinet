// Package handler реализует oapi.StrictServerInterface: переводит типы API
// в доменные и обратно и зовёт usecase. Логики здесь нет.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type Handler struct {
	health    usecase.HealthUseCase
	products  usecase.ProductUseCase
	companies usecase.CompanyUseCase
	apps      usecase.ApplicationUseCase
	auth      usecase.AuthUseCase

	sessionTTL   time.Duration
	cookieSecure bool
}

var _ oapi.StrictServerInterface = (*Handler)(nil)

type Deps struct {
	Health       usecase.HealthUseCase
	Products     usecase.ProductUseCase
	Companies    usecase.CompanyUseCase
	Applications usecase.ApplicationUseCase
	Auth         usecase.AuthUseCase

	// SessionTTL задаёт срок cookie, он совпадает со сроком сессии
	SessionTTL   time.Duration
	CookieSecure bool
}

func New(d Deps) *Handler {
	return &Handler{
		health:       d.Health,
		products:     d.Products,
		companies:    d.Companies,
		apps:         d.Applications,
		auth:         d.Auth,
		sessionTTL:   d.SessionTTL,
		cookieSecure: d.CookieSecure,
	}
}

func (h *Handler) Health(ctx context.Context, _ oapi.HealthRequestObject) (oapi.HealthResponseObject, error) {
	res := oapi.Health{Status: oapi.HealthStatusOk, Checks: map[string]oapi.HealthChecks{}}
	for name, err := range h.health.Check(ctx) {
		res.Checks[name] = oapi.HealthChecksOk
		if err != nil {
			res.Checks[name] = oapi.HealthChecksDown
			res.Status = oapi.HealthStatusDegraded
		}
	}
	if res.Status != oapi.HealthStatusOk {
		return oapi.Health503JSONResponse(res), nil
	}
	return oapi.Health200JSONResponse(res), nil
}

// Events отдаёт живые события по SSE, он вне strict server.
func (h *Handler) Events(echo.Context) error {
	return errNotImplemented
}

var errNotImplemented = echo.NewHTTPError(http.StatusNotImplemented)
