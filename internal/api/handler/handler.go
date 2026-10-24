// Package handler реализует oapi.StrictServerInterface: переводит типы API
// в доменные и обратно и зовёт usecase. Логики здесь нет.
package handler

import (
	"context"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

type Handler struct {
	health    usecase.HealthUseCase
	products  usecase.ProductUseCase
	companies usecase.CompanyUseCase
	apps      usecase.ApplicationUseCase
	auth      usecase.AuthUseCase
	events    usecase.EventUseCase
	docs      usecase.DocumentUseCase
	feed      usecase.FeedUseCase
	issue     usecase.IssueUseCase
	stats     usecase.StatsUseCase

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
	Events       usecase.EventUseCase
	Documents    usecase.DocumentUseCase
	Feed         usecase.FeedUseCase
	Issue        usecase.IssueUseCase
	Stats        usecase.StatsUseCase

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
		events:       d.Events,
		docs:         d.Documents,
		feed:         d.Feed,
		issue:        d.Issue,
		stats:        d.Stats,
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
