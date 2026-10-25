package handler

import (
	"context"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

const notificationsLimit = 50

func (h *Handler) ListNotifications(
	ctx context.Context, req oapi.ListNotificationsRequestObject,
) (oapi.ListNotificationsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	limit := notificationsLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	list, err := h.notifications.List(ctx, a.ID, req.Params.Unread != nil && *req.Params.Unread, limit)
	if err != nil {
		return nil, err
	}
	res := make(oapi.ListNotifications200JSONResponse, 0, len(list))
	for i := range list {
		n := &list[i]
		res = append(res, oapi.Notification{
			Id: n.ID, ApplicationId: n.ApplicationID, Kind: n.Kind, Title: n.Title, Text: n.Text,
			CreatedAt: n.CreatedAt, ReadAt: n.ReadAt,
		})
	}
	return res, nil
}

func (h *Handler) ReadNotification(
	ctx context.Context, req oapi.ReadNotificationRequestObject,
) (oapi.ReadNotificationResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.notifications.Read(ctx, a.ID, req.Id); err != nil {
		return nil, err
	}
	return oapi.ReadNotification204Response{}, nil
}

func (h *Handler) ReadAllNotifications(
	ctx context.Context, _ oapi.ReadAllNotificationsRequestObject,
) (oapi.ReadAllNotificationsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.notifications.ReadAll(ctx, a.ID); err != nil {
		return nil, err
	}
	return oapi.ReadAllNotifications204Response{}, nil
}
