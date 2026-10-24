package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func (h *Handler) ListFaults(
	ctx context.Context,
	_ oapi.ListFaultsRequestObject,
) (oapi.ListFaultsResponseObject, error) {
	fs, err := h.faults.List(ctx)
	if err != nil {
		return nil, err
	}
	res := make(oapi.ListFaults200JSONResponse, len(fs))
	for i, f := range fs {
		res[i] = oapi.SourceFault{Source: oapi.CheckSource(f.Source), Mode: oapi.SourceFaultMode(f.Mode)}
		if f.Left > 0 {
			res[i].Left = &f.Left
		}
	}
	return res, nil
}

func (h *Handler) SetFault(ctx context.Context, req oapi.SetFaultRequestObject) (oapi.SetFaultResponseObject, error) {
	if err := h.faults.Set(ctx, domain.Source(req.Body.Source), string(req.Body.Mode)); err != nil {
		return nil, err
	}
	return oapi.SetFault204Response{}, nil
}
