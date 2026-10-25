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
			Documents:            productDocs(p.Documents),
		})
	}
	return res, nil
}

func productDocs(rules []domain.DocumentRule) []oapi.ProductDocument {
	res := make([]oapi.ProductDocument, len(rules))
	for i, r := range rules {
		res[i] = oapi.ProductDocument{Kind: oapi.DocumentKind(r.Kind)}
		if r.When != "" {
			res[i].When = new(oapi.ProductDocumentWhen(r.When))
		}
		if r.AmountOverKop > 0 {
			res[i].AmountOverKop = &r.AmountOverKop
		}
	}
	return res
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
		return nil, err
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

func (h *Handler) NeedFromText(
	ctx context.Context,
	req oapi.NeedFromTextRequestObject,
) (oapi.NeedFromTextResponseObject, error) {
	n, err := h.products.Need(ctx, req.Body.Text)
	if err != nil {
		return nil, err
	}
	return oapi.NeedFromText200JSONResponse{
		Goal: oapi.Goal(n.Goal), AmountKop: opt(n.AmountKop), TermMonths: opt(n.TermMonths),
	}, nil
}
