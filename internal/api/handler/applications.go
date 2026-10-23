package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
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
	set(&d.PDConsent, b.PdConsent)
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
	v, err := h.card(ctx, a, req.Id)
	return oapi.GetApplication200JSONResponse(v), err
}

func (h *Handler) ListApplications(
	ctx context.Context,
	req oapi.ListApplicationsRequestObject,
) (oapi.ListApplicationsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p := req.Params
	f := domain.ApplicationFilter{Limit: pageSize}
	set(&f.Sort, (*string)(p.Sort))
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

func (h *Handler) ListApplicationEvents(
	ctx context.Context,
	req oapi.ListApplicationEventsRequestObject,
) (oapi.ListApplicationEventsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	events, err := h.apps.History(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	res := make(oapi.ListApplicationEvents200JSONResponse, 0, len(events))
	for i := range events {
		ev := &events[i]
		codes := make([]oapi.ReasonCode, 0, len(ev.Codes))
		for _, c := range ev.Codes {
			codes = append(codes, oapi.ReasonCode(c))
		}
		res = append(res, oapi.ApplicationEvent{
			Id:          ev.ID,
			Kind:        oapi.ApplicationEventKind(ev.Kind),
			FromStatus:  opt(oapi.ApplicationStatus(ev.From)),
			ToStatus:    opt(oapi.ApplicationStatus(ev.To)),
			ActorRole:   oapi.ActorRole(ev.ActorRole),
			ActorName:   opt(ev.ActorName),
			ReasonCodes: &codes,
			Comment:     opt(ev.Comment),
			CreatedAt:   ev.At,
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
		set(&d.PDConsent, b.PdConsent)
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

func (h *Handler) SubmitApplication(
	ctx context.Context,
	req oapi.SubmitApplicationRequestObject,
) (oapi.SubmitApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	app, err := h.apps.Submit(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	return oapi.SubmitApplication200JSONResponse(toView(&app, a.Role)), nil
}

func (h *Handler) ProvideInfo(
	ctx context.Context,
	req oapi.ProvideInfoRequestObject,
) (oapi.ProvideInfoResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	ans := usecase.Answer{RevenueKop: b.MonthlyRevenueKop}
	set(&ans.Documents, b.DocumentIds)
	set(&ans.Comment, b.Comment)
	err = h.apps.ProvideInfo(ctx, a, req.Id, ans)
	if err != nil {
		return nil, err
	}
	v, err := h.card(ctx, a, req.Id)
	return oapi.ProvideInfo200JSONResponse(v), err
}

func (h *Handler) RequestInfo(
	ctx context.Context,
	req oapi.RequestInfoRequestObject,
) (oapi.RequestInfoResponseObject, error) {
	// какие документы нужны, процесс пока не хранит, просьба уходит текстом
	v, err := h.act(ctx, req.Id, usecase.UpdateRequestInfo, req.Body.Comment)
	return oapi.RequestInfo200JSONResponse(v), err
}

func (h *Handler) RetryChecks(
	ctx context.Context,
	req oapi.RetryChecksRequestObject,
) (oapi.RetryChecksResponseObject, error) {
	v, err := h.act(ctx, req.Id, usecase.UpdateRetryChecks, "")
	return oapi.RetryChecks200JSONResponse(v), err
}

func (h *Handler) DecideApplication(
	ctx context.Context,
	req oapi.DecideApplicationRequestObject,
) (oapi.DecideApplicationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	act := domain.Action{Actor: a, Outcome: domain.Status(b.Outcome), Comment: b.Comment}
	if b.ReasonCodes != nil {
		for _, c := range *b.ReasonCodes {
			act.Codes = append(act.Codes, string(c))
		}
	}
	if t := b.Terms; t != nil {
		act.Terms = &domain.Terms{
			Product: domain.ProductCode(t.Product), AmountKop: t.AmountKop, TermMonths: t.TermMonths, RateBP: t.RateBp,
		}
		set(&act.Terms.Guarantor, t.GuarantorRequired)
		set(&act.Terms.Pledge, t.PledgeRequired)
	}
	err = h.apps.Decide(ctx, req.Id, act)
	if err != nil {
		return nil, err
	}
	v, err := h.card(ctx, a, req.Id)
	return oapi.DecideApplication200JSONResponse(v), err
}

// act передаёт действие процессу и отдаёт карточку уже после перехода
func (h *Handler) act(ctx context.Context, id uuid.UUID, update, comment string) (oapi.ApplicationView, error) {
	a, err := actor(ctx)
	if err != nil {
		return oapi.ApplicationView{}, err
	}
	err = h.apps.Act(ctx, id, update, domain.Action{Actor: a, Comment: comment})
	if err != nil {
		return oapi.ApplicationView{}, err
	}
	return h.card(ctx, a, id)
}

func (h *Handler) card(ctx context.Context, a domain.Actor, id uuid.UUID) (oapi.ApplicationView, error) {
	app, err := h.apps.Get(ctx, a, id)
	if err != nil {
		return oapi.ApplicationView{}, err
	}
	return toView(&app, a.Role), nil
}

func actor(ctx context.Context) (domain.Actor, error) {
	s, ok := middleware.ActorFrom(ctx)
	if !ok {
		return domain.Actor{}, domain.ErrUnauthorized
	}
	return domain.Actor{ID: s.UserID, Role: s.Role}, nil
}

// toView: причины и условия появятся в карточке вместе с подачей и решением
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
		PdConsent:          app.PDConsent,
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
	for i := range app.Documents {
		d := &app.Documents[i]
		v.Documents = append(v.Documents, oapi.DocumentRef{
			Id: d.ID, Kind: oapi.DocumentKind(d.Kind), Status: oapi.DocumentStatus(d.Status), FileName: d.FileName,
		})
	}
	if app.Grade != "" {
		g := oapi.Grade(app.Grade)
		v.Grade = &g
	}
	if d := app.Decision; d != nil {
		decisionView(&v, d, app.DueAt, role)
	}
	if role != domain.RoleClient {
		checks := make([]oapi.CheckResult, 0, len(app.Checks))
		for _, c := range app.Checks {
			r := oapi.CheckResult{
				Source: oapi.CheckSource(c.Source), Status: oapi.CheckStatus(c.Status),
				Error: opt(c.Error), CheckedAt: c.At,
			}
			// payload из jsonb, разбор не падает
			if c.Payload != nil {
				_ = json.Unmarshal(c.Payload, &r.Payload)
			}
			checks = append(checks, r)
		}
		v.Checks = &checks
	}
	return v
}

// decisionView: клиент видит условия и причины с текстом, банк ещё коды без текста,
// балл с разбивкой и рекомендацию политики
func decisionView(v *oapi.ApplicationView, d *domain.Decision, due *time.Time, role domain.Role) {
	bank := role != domain.RoleClient
	for _, r := range d.Reasons {
		if r.Text != "" || bank {
			v.Reasons = append(v.Reasons, oapi.Reason{Code: oapi.ReasonCode(r.Code), Text: r.Text})
		}
	}
	if t := d.Terms; t != nil {
		o := oapi.Offer{
			Product: oapi.ProductCode(t.Product), AmountKop: t.AmountKop, TermMonths: t.TermMonths, RateBp: t.RateBP,
			MonthlyPaymentKop: t.PaymentKop, GuarantorRequired: &t.Guarantor, PledgeRequired: &t.Pledge,
		}
		if due != nil {
			o.ExpiresAt = *due
		}
		v.Offer = &o
	}
	if !bank {
		return
	}
	v.PolicyVersion = opt(d.PolicyVersion)
	if t := d.Recommendation; t != nil {
		v.Recommendation = &oapi.OfferTerms{
			Product: oapi.ProductCode(t.Product), AmountKop: t.AmountKop, TermMonths: t.TermMonths, RateBp: t.RateBP,
			GuarantorRequired: &t.Guarantor, PledgeRequired: &t.Pledge,
		}
	}
	if s := d.Score; s != nil {
		sc := oapi.Scoring{Score: s.Points, Grade: oapi.Grade(s.Grade), Pd: s.PD, Version: s.Version}
		for _, f := range s.Factors {
			sc.Factors = append(sc.Factors, oapi.ScoreFactor{
				Code: f.Code, Name: f.Name, Value: float32(f.Value), Points: f.Points, MaxPoints: f.Max,
				Reason: opt(oapi.ReasonCode(f.Reason)),
			})
		}
		v.Scoring = &sc
	}
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
