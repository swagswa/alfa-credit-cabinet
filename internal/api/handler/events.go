package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

// keepAlive короче proxy_read_timeout у nginx и таймаутов прокси по дороге
const keepAlive = 20 * time.Second

// Events отдаёт живые события по SSE. Он вне strict server: тот стримить не умеет.
func (h *Handler) Events(c echo.Context) error {
	ctx := c.Request().Context()
	s, _ := middleware.ActorFrom(ctx)
	var appID uuid.UUID
	if v := c.QueryParam("application_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "application_id: нужен uuid")
		}
		appID = id
	}

	events, cancel, err := h.events.Subscribe(ctx, s, appID)
	if err != nil {
		return err
	}
	defer cancel()

	w := c.Response()
	w.Header().Set(echo.HeaderContentType, "text/event-stream")
	w.Header().Set(echo.HeaderCacheControl, "no-cache")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	ping := time.NewTicker(keepAlive)
	defer ping.Stop()
	for {
		// flush падает, когда клиент уже отключился
		if err := rc.Flush(); err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(oapi.StreamEvent{
				ApplicationId: ev.ApplicationID,
				Status:        new(oapi.ApplicationStatus(ev.Status)),
				Version:       ev.Version,
			})
			_, _ = fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
		}
	}
}
