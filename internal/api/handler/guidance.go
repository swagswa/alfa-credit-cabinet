package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

func (h *Handler) ApplicationImprovements(
	ctx context.Context,
	req oapi.ApplicationImprovementsRequestObject,
) (oapi.ApplicationImprovementsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	app, err := h.apps.Get(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	p := h.guidance.Improvements(&app)
	res := oapi.ApplicationImprovements200JSONResponse{
		Available: p.Available,
		Score:     p.Score,
		Grade:     p.Grade,
		Version:   p.Version,
	}
	res.Items = []oapi.Improvement{}
	for _, step := range p.Items {
		res.Items = append(res.Items, oapi.Improvement{Input: step.Input, Title: step.Title, Detail: step.Detail,
			Target: step.Target, Delta: step.Delta, Score: step.Score, Grade: oapi.Grade(step.Grade)})
	}
	return res, nil
}

func (h *Handler) PreliminaryEstimate(
	ctx context.Context,
	req oapi.PreliminaryEstimateRequestObject,
) (oapi.PreliminaryEstimateResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.guidance.Preliminary(ctx, a, req.Inn)
	if err != nil {
		return nil, err
	}
	res := oapi.PreliminaryEstimate200JSONResponse{Version: p.Version, MonthlyRevenueKop: p.MonthlyRevenueKop,
		Sources: []oapi.PreliminaryEstimateSources{}, Items: []oapi.PreliminaryAmount{}}
	for _, source := range p.Sources {
		res.Sources = append(res.Sources, oapi.PreliminaryEstimateSources(source))
	}
	for _, item := range p.Items {
		res.Items = append(
			res.Items,
			oapi.PreliminaryAmount{Product: oapi.ProductCode(item.Product), AmountKop: item.AmountKop},
		)
	}
	return res, nil
}
