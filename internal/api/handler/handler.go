// Package handler реализует oapi.StrictServerInterface: переводит типы API
// в доменные и обратно и зовёт usecase. Логики здесь нет.
package handler

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type Handler struct {
	health    usecase.HealthUseCase
	products  usecase.ProductUseCase
	companies usecase.CompanyUseCase
}

var _ oapi.StrictServerInterface = (*Handler)(nil)

func New(health usecase.HealthUseCase, products usecase.ProductUseCase, companies usecase.CompanyUseCase) *Handler {
	return &Handler{health: health, products: products, companies: companies}
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
