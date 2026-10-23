package handler

// Операции, которые пока не реализованы. С каждой фичей метод переезжает
// в файл своей области и получает тело.

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

func (h *Handler) AcceptOffer(context.Context, oapi.AcceptOfferRequestObject) (oapi.AcceptOfferResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) SignApplication(
	context.Context,
	oapi.SignApplicationRequestObject,
) (oapi.SignApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) RequestSigningCode(
	context.Context,
	oapi.RequestSigningCodeRequestObject,
) (oapi.RequestSigningCodeResponseObject, error) {
	return nil, errNotImplemented
}
