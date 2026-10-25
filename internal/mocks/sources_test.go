package mocks_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

const consentID = `"consent_id":"01930000-0000-7000-8000-0000000000aa"`

// согласие, полученное months месяцев назад
func consentAgo(months int) string {
	return fmt.Sprintf(`%s,"consent_given_at":%q`, consentID, time.Now().AddDate(0, -months, 0).Format(time.RFC3339))
}

var consent = consentAgo(1)

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

// field достаёт значение по пути вида "summary.inquiries_90d" или "loans.0.amount"
func field(m map[string]any, path string) any {
	var v any = m
	for k := range strings.SplitSeq(path, ".") {
		switch x := v.(type) {
		case map[string]any:
			v = x[k]
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
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
		{"просрочка автосервиса", "POST", "/bki/report", `{"inn":"0000700010",` + consent + `}`, 200, map[string]any{
			"loans.0.current_dpd": 95.0, "owner.current_dpd": 0.0,
		}},
		{"бки без согласия", "POST", "/bki/report", `{"inn":"0000100012"}`, 403, nil},
		{"согласие без даты", "POST", "/bki/report", `{"inn":"0000100012",` + consentID + `}`, 403, nil},
		{"согласие истекло", "POST", "/bki/report", `{"inn":"0000100012",` + consentAgo(7) + `}`, 403, nil},
		{"кривой id согласия", "POST", "/bki/report", `{"inn":"0000100012","consent_id":"x"}`, 400, nil},
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

// Источники говорят об одной компании, выписка из генератора документов сходится с ФНС и банком.
// Демо плюс тысяча ИНН из генератора, каждый третий ИП.
func TestSourcesMatchSynth(t *testing.T) {
	e := mocks.New()
	inns := []string{"0000100012", "000020001010", "0000300011", "0000400016", "0000500010", "0000700010"}
	rng := synth.RNG("inns")
	for i := range 1000 {
		base := fmt.Sprintf("%09d", rng.IntN(1e9))
		if i%3 == 0 {
			base += strconv.Itoa(rng.IntN(10))
		}
		inns = append(inns, domain.CompleteINN(base))
	}

	for i, inn := range inns {
		t.Run(inn, func(t *testing.T) {
			c, err := synth.New(inn)
			require.NoError(t, err)
			in, _ := c.Turnover()
			income := float64(c.Income() / domain.KopInRub)

			code, reg := call(t, e, "GET", "/egrul/"+inn, "")
			if inn != "0000500010" { // у неё егрюл лежит
				require.Equal(t, http.StatusOK, code)
				require.Equal(t, inn, reg["inn"])
				if c.IsIP() {
					require.Equal(
						t,
						"Индивидуальный предприниматель "+strings.TrimPrefix(c.Name, "ИП "),
						reg["full_name"],
					)
				} else {
					require.Equal(t, c.Name, reg["short_name"])
					require.Equal(t, field(reg, "director.inn"), field(reg, "founders.0.inn"))
				}
			}

			code, fns := call(t, e, "GET", "/fns/"+inn, "")
			require.Equal(t, http.StatusOK, code)
			require.InDelta(t, income, field(fns, "income_last_year"), 0)
			if i%20 == 0 { // pdf рисуется не мгновенно, хватит каждого двадцатого
				_, truth, err := docgen.Generate(docgen.Doc{INN: inn, Kind: docgen.KindStatement, Format: "a"})
				require.NoError(t, err)
				var st struct {
					TurnoverIn float64 `json:"turnover_in"`
				}
				require.NoError(t, json.Unmarshal(truth, &st))
				require.InDelta(t, income, st.TurnoverIn, 1, "выписка и ФНС")
			}

			code, bki := call(t, e, "POST", "/bki/report", `{"inn":"`+inn+`",`+consent+`}`)
			require.Equal(t, http.StatusOK, code)
			require.LessOrEqual(
				t,
				field(bki, "summary.monthly_payments_total").(float64),
				income/12,
				"платежи больше выручки",
			)

			code, bank := call(t, e, "GET", "/internal/"+inn, "")
			if code == http.StatusNotFound { // счёта у нас нет
				return
			}
			require.Equal(t, http.StatusOK, code)
			require.Equal(t, true, bank["is_client"])
			var credit float64
			for _, m := range bank["turnover_12m"].([]any) {
				credit += m.(map[string]any)["credit"].(float64)
			}
			require.InDelta(t, float64(in/domain.KopInRub), credit, 12, "копейки отбрасываем помесячно")
			require.Regexp(t, `^40[78]02810\d{12}$`, bank["account"], "счёт тот же, что в выписке")
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
