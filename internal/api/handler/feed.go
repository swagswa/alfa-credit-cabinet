package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

const feedSize = 30

func (h *Handler) ListFeed(ctx context.Context, req oapi.ListFeedRequestObject) (oapi.ListFeedResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var company uuid.UUID
	set(&company, req.Params.CompanyId)
	limit := feedSize
	set(&limit, req.Params.Limit)

	items, err := h.feed.List(ctx, a, company, limit)
	if err != nil {
		return nil, err
	}
	res := make(oapi.ListFeed200JSONResponse, 0, len(items))
	for i := range items {
		it := &items[i]
		res = append(res, oapi.FeedItem{
			Id:            it.ID,
			Kind:          oapi.FeedItemKind(it.Kind),
			Title:         it.Title,
			Text:          it.Text,
			ApplicationId: opt(it.ApplicationID),
			CreatedAt:     it.At,
		})
	}
	return res, nil
}
