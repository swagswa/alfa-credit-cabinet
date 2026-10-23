package source_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/source"
)

const inn = "0000100012"

func TestFetch(t *testing.T) {
	consent := domain.BKIConsent{
		ID:      uuid.MustParse("01930000-0000-7000-8000-0000000000aa"),
		GivenAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name     string
		src      domain.Source
		status   int
		delay    time.Duration
		wantReq  string // метод, путь и тело, как их увидел мок
		wantBody string
		err      error
	}{
		{"егрюл", domain.SourceEGRUL, http.StatusOK, 0, "GET /egrul/0000100012 ", `{"status":"active"}`, nil},
		{"фнс", domain.SourceFNS, http.StatusOK, 0, "GET /fns/0000100012 ", `{"status":"active"}`, nil},
		{
			"данные банка",
			domain.SourceInternal,
			http.StatusOK,
			0,
			"GET /internal/0000100012 ",
			`{"status":"active"}`,
			nil,
		},
		{
			"бки с согласием",
			domain.SourceBKI,
			http.StatusOK,
			0,
			`POST /bki/report {"inn":"0000100012","consent_id":"01930000-0000-7000-8000-0000000000aa",` +
				`"consent_given_at":"2026-10-01T10:00:00Z"}`,
			`{"status":"active"}`,
			nil,
		},
		{"нет в реестре", domain.SourceEGRUL, http.StatusNotFound, 0, "", "", domain.ErrNotFound},
		{"источник лежит", domain.SourceFNS, http.StatusServiceUnavailable, 0, "", "", domain.ErrUnavailable},
		{"бки без согласия", domain.SourceBKI, http.StatusForbidden, 0, "", "", domain.ErrForbidden},
		{"кривой запрос", domain.SourceInternal, http.StatusBadRequest, 0, "", "", domain.ErrInvalid},
		{"не дождались", domain.SourceEGRUL, http.StatusOK, 300 * time.Millisecond, "", "", domain.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				got = r.Method + " " + r.URL.Path + " " + string(body)
				time.Sleep(tt.delay)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"status":"active"}`))
			}))
			defer srv.Close()

			c := source.New(
				source.URLs{EGRUL: srv.URL, FNS: srv.URL, BKI: srv.URL, Bank: srv.URL},
				100*time.Millisecond,
			)
			payload, err := c.Fetch(t.Context(), tt.src, inn, consent)

			require.ErrorIs(t, err, tt.err)
			if tt.err == nil {
				require.Equal(t, tt.wantReq, got)
				require.JSONEq(t, tt.wantBody, string(payload))
			}
		})
	}
}
