package handler

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

func (h *Handler) LookupCompany(
	ctx context.Context,
	req oapi.LookupCompanyRequestObject,
) (oapi.LookupCompanyResponseObject, error) {
	c, err := h.companies.Lookup(ctx, req.Inn)
	if err != nil {
		return nil, err //nolint:wrapcheck // в HTTP переводит errorHandler
	}
	res := oapi.LookupCompany200JSONResponse{
		Inn:          c.INN,
		Kind:         oapi.CompanyKind(c.Kind),
		Ogrn:         c.OGRN,
		FullName:     c.FullName,
		ShortName:    c.ShortName,
		Status:       oapi.CompanyStatus(c.Status),
		RegisteredAt: openapi_types.Date{Time: c.RegisteredAt},
		Address:      c.Address,
		OkvedMain:    oapi.Okved{Code: c.Okved.Code, Name: c.Okved.Name},
		Simulated:    c.Simulated,
	}
	if c.KPP != "" {
		res.Kpp = &c.KPP
	}
	if c.Director != nil {
		res.Director = &oapi.Director{Fio: c.Director.FIO, Position: c.Director.Position}
	}
	if len(c.Founders) > 0 {
		fs := make([]oapi.Founder, 0, len(c.Founders))
		for _, f := range c.Founders {
			fs = append(fs, oapi.Founder{Name: f.Name, SharePct: float32(f.SharePct)})
		}
		res.Founders = &fs
	}
	return res, nil
}
