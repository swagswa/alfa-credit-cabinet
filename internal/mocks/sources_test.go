package mocks_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

const consent = `"consent_id":"01930000-0000-7000-8000-0000000000aa"`

func call(t *testing.T, e *echo.Echo, method, path, body string) (code int, got map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got), rec.Body.String())
	}
	return rec.Code, got
}

// field достаёт значение по пути вида "summary.inquiries_90d"
func field(m map[string]any, path string) any {
	var v any = m
	for k := range strings.SplitSeq(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[k]
	}
	return v
}

func TestSources(t *testing.T) {
	tests := []struct {
		name         string
		method, path string
		body         string
		code         int
		want         map[string]any // путь в ответе: значение
	}{
		{"кофейня в егрюл", "GET", "/egrul/0000100012", "", 200, map[string]any{
			"short_name": "ООО «Тёплое зерно»", "status": "active", "registered_at": "2023-06-15",
			"okved_main.code": "56.30", "director.fio": "Соколова Анна Викторовна",
		}},
		{"ип в егрип без кпп и директора", "GET", "/egrul/000020001010", "", 200, map[string]any{
			"kind": "IP", "kpp": nil, "director": nil,
		}},
		{"недоимка it-компании", "GET", "/fns/0000300011", "", 200, map[string]any{"arrears.tax": 30000.0}},
		{"бки кофейни", "POST", "/bki/report", `{"inn":"0000100012",` + consent + `}`, 200, map[string]any{
			"summary.monthly_payments_total": 60000.0, "summary.inquiries_90d": 1.0,
		}},
		{"просрочка стройки", "POST", "/bki/report", `{"inn":"0000400016",` + consent + `}`, 200, map[string]any{
			"summary.monthly_payments_total": 1400000.0, "summary.inquiries_90d": 9.0,
		}},
		{"бки без согласия", "POST", "/bki/report", `{"inn":"0000100012"}`, 403, nil},
		{"кофейня клиент банка", "GET", "/internal/0000100012", "", 200, map[string]any{"is_client": true}},
		{"у it-компании нет счёта", "GET", "/internal/0000300011", "", 404, nil},
		{"егрюл тихой компании лежит", "GET", "/egrul/0000500010", "", 503, nil},
		{"а фнс по ней отвечает", "GET", "/fns/0000500010", "", 200, nil},
		{"инн не из демо из генератора", "GET", "/egrul/7701234560", "", 200, map[string]any{
			"inn": "7701234560", "address.region_code": "77", "generator_version": "1",
		}},
		{"кривой инн как несуществующий", "GET", "/fns/123", "", 404, nil},
		{"не те контрольные цифры", "POST", "/bki/report", `{"inn":"0000100013",` + consent + `}`, 404, nil},
	}
	e := mocks.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, got := call(t, e, tt.method, tt.path, tt.body)
			require.Equal(t, tt.code, code)
			for path, want := range tt.want {
				require.Equal(t, want, field(got, path), path)
			}
		})
	}
}

// выписка и отчётность из генератора документов должны сходиться с ФНС и банком
func TestSourcesMatchSynth(t *testing.T) {
	e := mocks.New()
	for _, inn := range []string{"0000100012", "000020001010", "0000300011", "0000400016", "0000500010"} {
		t.Run(inn, func(t *testing.T) {
			c, err := synth.New(inn)
			require.NoError(t, err)
			in, _ := c.Turnover()

			_, fns := call(t, e, "GET", "/fns/"+inn, "")
			require.InDelta(t, float64(c.Income()/domain.KopInRub), field(fns, "income_last_year"), 0)

			code, bank := call(t, e, "GET", "/internal/"+inn, "")
			if code == http.StatusNotFound {
				return
			}
			var credit float64
			for _, m := range bank["turnover_12m"].([]any) {
				credit += m.(map[string]any)["credit"].(float64)
			}
			require.InDelta(t, float64(in/domain.KopInRub), credit, 12, "копейки отбрасываем помесячно")
		})
	}
}

func TestFaults(t *testing.T) {
	e := mocks.New()
	set := func(body string) {
		code, _ := call(t, e, "PUT", "/mocks/admin/faults", body)
		require.Equal(t, http.StatusNoContent, code, body)
	}
	fns := func() int { code, _ := call(t, e, "GET", "/fns/0000100012", ""); return code }

	set(`{"source":"fns","mode":"http_500"}`)
	require.Equal(t, 500, fns())
	set(`{"source":"fns","mode":"ok"}`)
	require.Equal(t, 200, fns())

	// flaky: первые два вызова падают, дальше ответ
	set(`{"source":"fns","mode":"flaky","n":2}`)
	require.Equal(t, []int{500, 500, 200, 200}, []int{fns(), fns(), fns(), fns()})

	// режим на одну компанию не трогает остальные
	set(`{"source":"egrul","mode":"not_found","inn":"0000100012"}`)
	code, _ := call(t, e, "GET", "/egrul/0000100012", "")
	require.Equal(t, 404, code)
	code, _ = call(t, e, "GET", "/egrul/0000300011", "")
	require.Equal(t, 200, code)

	code, _ = call(t, e, "PUT", "/mocks/admin/faults", `{"source":"fns","mode":"explode"}`)
	require.Equal(t, 400, code)
	code, _ = call(t, e, "PUT", "/mocks/admin/faults", `{"source":"cbr","mode":"ok"}`)
	require.Equal(t, 400, code)
}

// timeout держит ответ, пока клиент не бросит запрос
func TestFaultTimeout(t *testing.T) {
	e := mocks.New()
	code, _ := call(t, e, "PUT", "/mocks/admin/faults", `{"source":"bki","mode":"timeout"}`)
	require.Equal(t, http.StatusNoContent, code)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(
		ctx,
		"POST",
		"/bki/report",
		strings.NewReader(`{"inn":"0000100012",`+consent+`}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	start := time.Now()
	e.ServeHTTP(rec, req)
	require.Less(t, time.Since(start), time.Second)
	require.NotEqual(t, http.StatusOK, rec.Code)
}
