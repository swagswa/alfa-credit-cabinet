package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const pageSize = 20

func (h *Handler) CreateApplication(
	ctx context.Context,
	req oapi.CreateApplicationRequestObject,
) (oapi.CreateApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	d := domain.Draft{
		Role:       domain.RepresentativeRole(b.RepresentativeRole),
		Product:    domain.ProductCode(b.Product),
		Purpose:    domain.Goal(b.Purpose),
		AmountKop:  b.AmountKop,
		TermMonths: b.TermMonths,
	}
	set(&d.MonthlyRevenueKop, b.MonthlyRevenueKop)
	set(&d.EquipmentCostKop, b.EquipmentCostKop)
	set(&d.BKIConsent, b.BkiConsent)
	set(&d.Comment, b.Comment)
	app, err := h.apps.Create(ctx, a, req.Params.IdempotencyKey, b.Inn, d)
	if err != nil {
		return nil, err
	}
	return oapi.CreateApplication201JSONResponse(toView(&app, a.Role)), nil
}

func (h *Handler) GetApplication(
	ctx context.Context,
	req oapi.GetApplicationRequestObject,
) (oapi.GetApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	app, err := h.apps.Get(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	return oapi.GetApplication200JSONResponse(toView(&app, a.Role)), nil
}

func (h *Handler) ListApplications(
	ctx context.Context,
	req oapi.ListApplicationsRequestObject,
) (oapi.ListApplicationsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	// sort пока не разбираем, всегда новые сверху; остальные порядки с очередью банка
	p := req.Params
	f := domain.ApplicationFilter{Limit: pageSize}
	set(&f.Limit, p.Limit)
	set(&f.Offset, p.Offset)
	set(&f.Product, (*domain.ProductCode)(p.Product))
	if p.Status != nil {
		for _, s := range *p.Status {
			f.Statuses = append(f.Statuses, domain.Status(s))
		}
	}
	if p.Grade != nil {
		for _, g := range *p.Grade {
			f.Grades = append(f.Grades, string(g))
		}
	}
	apps, total, err := h.apps.List(ctx, a, f)
	if err != nil {
		return nil, err
	}
	res := oapi.ListApplications200JSONResponse{Total: total, Items: make([]oapi.ApplicationSummary, 0, len(apps))}
	for i := range apps {
		v := toView(&apps[i], a.Role)
		res.Items = append(res.Items, oapi.ApplicationSummary{
			Id: v.Id, Number: v.Number, Status: v.Status, Company: v.Company, Product: v.Product,
			AmountKop: v.AmountKop, TermMonths: v.TermMonths, Grade: v.Grade, Score: v.Score, DueAt: v.DueAt,
			CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		})
	}
	return res, nil
}

func (h *Handler) UpdateApplication(
	ctx context.Context,
	req oapi.UpdateApplicationRequestObject,
) (oapi.UpdateApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	app, err := h.apps.Edit(ctx, a, req.Id, b.Version, func(d *domain.Draft) {
		set(&d.Role, (*domain.RepresentativeRole)(b.RepresentativeRole))
		set(&d.Product, (*domain.ProductCode)(b.Product))
		set(&d.Purpose, (*domain.Goal)(b.Purpose))
		set(&d.AmountKop, b.AmountKop)
		set(&d.TermMonths, b.TermMonths)
		set(&d.MonthlyRevenueKop, b.MonthlyRevenueKop)
		set(&d.EquipmentCostKop, b.EquipmentCostKop)
		set(&d.BKIConsent, b.BkiConsent)
		set(&d.Comment, b.Comment)
	})
	if err != nil {
		return nil, err
	}
	return oapi.UpdateApplication200JSONResponse(toView(&app, a.Role)), nil
}

func (h *Handler) CancelApplication(
	ctx context.Context,
	req oapi.CancelApplicationRequestObject,
) (oapi.CancelApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var reason string
	if req.Body != nil {
		set(&reason, req.Body.Reason)
	}
	app, err := h.apps.Cancel(ctx, a, req.Id, reason)
	if err != nil {
		return nil, err
	}
	return oapi.CancelApplication200JSONResponse(toView(&app, a.Role)), nil
}

func actor(ctx context.Context) (domain.Actor, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	return domain.Actor{ID: s.UserID, Role: s.Role}, nil
}

// toView: документы, причины и условия появятся в карточке вместе с подачей и решением
func toView(app *domain.Application, role domain.Role) oapi.ApplicationView {
	next := []oapi.ApplicationStatus{}
	for _, s := range domain.AllowedNext(app.Status, role) {
		next = append(next, oapi.ApplicationStatus(s))
	}
	v := oapi.ApplicationView{
		Id:     app.ID,
		Number: app.Number,
		Status: oapi.ApplicationStatus(app.Status),
		Company: oapi.CompanyRef{
			Id:        app.CompanyID,
			Inn:       app.Company.INN,
			Kind:      oapi.CompanyKind(app.Company.Kind),
			ShortName: app.Company.ShortName,
			Simulated: app.Company.Simulated,
		},
		Version:            app.Version,
		RepresentativeRole: oapi.RepresentativeRole(app.Role),
		Product:            oapi.ProductCode(app.Product),
		Purpose:            oapi.Goal(app.Purpose),
		AmountKop:          app.AmountKop,
		TermMonths:         app.TermMonths,
		MonthlyRevenueKop:  opt(app.MonthlyRevenueKop),
		EquipmentCostKop:   opt(app.EquipmentCostKop),
		BkiConsent:         app.BKIConsent,
		Comment:            opt(app.Comment),
		Score:              app.Score,
		DueAt:              app.DueAt,
		SubmittedAt:        app.SubmittedAt,
		CreatedAt:          app.CreatedAt,
		UpdatedAt:          app.UpdatedAt,
		AllowedNext:        next,
		Reasons:            []oapi.Reason{},
		Documents:          []oapi.DocumentRef{},
	}
	if app.Grade != "" {
		g := oapi.Grade(app.Grade)
		v.Grade = &g
	}
	return v
}

// set переносит необязательное поле запроса, если оно пришло
func set[T any](dst, src *T) {
	if src != nil {
		*dst = *src
	}
}

func opt[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
