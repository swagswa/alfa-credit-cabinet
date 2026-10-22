package handler

// Операции, которые пока не реализованы. С каждой фичей метод переезжает
// в файл своей области и получает тело.

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

func (h *Handler) ListApplications(
	context.Context,
	oapi.ListApplicationsRequestObject,
) (oapi.ListApplicationsResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) CreateApplication(
	context.Context,
	oapi.CreateApplicationRequestObject,
) (oapi.CreateApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) GetApplication(
	context.Context,
	oapi.GetApplicationRequestObject,
) (oapi.GetApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) UpdateApplication(
	context.Context,
	oapi.UpdateApplicationRequestObject,
) (oapi.UpdateApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) AcceptOffer(context.Context, oapi.AcceptOfferRequestObject) (oapi.AcceptOfferResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) CancelApplication(
	context.Context,
	oapi.CancelApplicationRequestObject,
) (oapi.CancelApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) DecideApplication(
	context.Context,
	oapi.DecideApplicationRequestObject,
) (oapi.DecideApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) UploadDocument(
	context.Context,
	oapi.UploadDocumentRequestObject,
) (oapi.UploadDocumentResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) ListApplicationEvents(
	context.Context,
	oapi.ListApplicationEventsRequestObject,
) (oapi.ListApplicationEventsResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) ProvideInfo(context.Context, oapi.ProvideInfoRequestObject) (oapi.ProvideInfoResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) RequestInfo(context.Context, oapi.RequestInfoRequestObject) (oapi.RequestInfoResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) RetryChecks(context.Context, oapi.RetryChecksRequestObject) (oapi.RetryChecksResponseObject, error) {
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

func (h *Handler) SubmitApplication(
	context.Context,
	oapi.SubmitApplicationRequestObject,
) (oapi.SubmitApplicationResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) GetDocument(context.Context, oapi.GetDocumentRequestObject) (oapi.GetDocumentResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) ConfirmDocument(
	context.Context,
	oapi.ConfirmDocumentRequestObject,
) (oapi.ConfirmDocumentResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) DownloadDocument(
	context.Context,
	oapi.DownloadDocumentRequestObject,
) (oapi.DownloadDocumentResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) ListFeed(context.Context, oapi.ListFeedRequestObject) (oapi.ListFeedResponseObject, error) {
	return nil, errNotImplemented
}

func (h *Handler) GetSampleDocument(
	context.Context,
	oapi.GetSampleDocumentRequestObject,
) (oapi.GetSampleDocumentResponseObject, error) {
	return nil, errNotImplemented
}
