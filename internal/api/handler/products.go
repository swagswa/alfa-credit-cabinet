package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

func (h *Handler) ListProducts(
	context.Context,
	oapi.ListProductsRequestObject,
) (oapi.ListProductsResponseObject, error) {
	ps := h.products.List()
	res := make(oapi.ListProducts200JSONResponse, 0, len(ps))
	for i := range ps {
		p := &ps[i]
		res = append(res, oapi.Product{
			Code:                 oapi.ProductCode(p.Code),
			Name:                 p.Name,
			Description:          p.Description,
			AmountMinKop:         p.AmountMinKop,
			AmountMaxKop:         p.AmountMaxKop,
			TermMinMonths:        p.TermMinMonths,
			TermMaxMonths:        p.TermMaxMonths,
			RateMinBp:            p.BaseRateBP,
			RateMaxBp:            p.MaxRateBP,
			Repayment:            oapi.ProductRepayment(p.Repayment),
			MinBusinessAgeMonths: p.MinBusinessAgeMonths,
			Requirements:         p.Requirements,
		})
	}
	return res, nil
}

func (h *Handler) RecommendProducts(
	_ context.Context,
	req oapi.RecommendProductsRequestObject,
) (oapi.RecommendProductsResponseObject, error) {
	q := usecase.RecommendQuery{
		Goal:       domain.Goal(req.Params.Goal),
		AmountKop:  req.Params.AmountKop,
		TermMonths: req.Params.TermMonths,
	}
	if req.Params.MonthlyRevenueKop != nil {
		q.MonthlyRevenueKop = *req.Params.MonthlyRevenueKop
	}
	offers, err := h.products.Recommend(q)
	if err != nil {
		return nil, err //nolint:wrapcheck // в HTTP переводит errorHandler
	}
	res := oapi.RecommendProducts200JSONResponse{
		Goal:  req.Params.Goal,
		Items: make([]oapi.RecommendationItem, 0, len(offers)),
	}
	for _, o := range offers {
		res.Items = append(res.Items, oapi.RecommendationItem{
			Product:           oapi.ProductCode(o.Product),
			Primary:           o.Primary,
			AmountKop:         o.AmountKop,
			TermMonths:        o.TermMonths,
			RateBp:            o.RateBP,
			MonthlyPaymentKop: o.MonthlyPaymentKop,
		})
	}
	return res, nil
}
