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
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
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
			se, name := streamEvent(ev, s.Role)
			data, _ := json.Marshal(se)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
		}
	}
}

// streamEvent: имя события SSE по тому, что в нём поменялось
func streamEvent(ev domain.Event, role domain.Role) (se oapi.StreamEvent, name string) {
	se = oapi.StreamEvent{
		ApplicationId: ev.ApplicationID,
		Status:        new(oapi.ApplicationStatus(ev.Status)),
		Version:       ev.Version,
	}
	switch {
	case ev.DocumentID != uuid.Nil:
		se.DocumentId, se.DocumentStatus = &ev.DocumentID, new(oapi.DocumentStatus(ev.DocumentStatus))
		return se, "document"
	case ev.Source != "":
		se.Source = new(oapi.CheckSource(ev.Source))
		// у записи «так и не ответил» попытки нет, это отдельная activity после всех повторов
		if ev.Attempt > 0 {
			se.Attempt, se.Attempts = &ev.Attempt, &ev.Attempts
		}
		if ev.CheckStatus != "" {
			se.CheckStatus = new(oapi.CheckStatus(ev.CheckStatus))
		}
		// причину сбоя видит только банк, как и проверки в карточке
		if ev.CheckError != "" && role != domain.RoleClient {
			se.Error = &ev.CheckError
		}
		return se, "check"
	}
	return se, "status"
}
