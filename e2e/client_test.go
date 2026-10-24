//go:build e2e

// Сценарии приёмки через настоящий api на поднятом стенде: make up && make e2e.
// Каждый прогон заводит новые заявки, чистить базу между прогонами не нужно.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
)

const docs = "../testdata/documents/"

var (
	baseURL = env("E2E_API_URL", "http://localhost:8080/api")
	// сроки демо из compose; в CI стенд поднимается с короткими и ждать можно меньше
	waitFor = durationEnv("E2E_WAIT", 90*time.Second)

	mu    sync.Mutex
	users = map[string]*user{}
)

// user клиент контракта с cookie сессии; http нужен для SSE, его генератор не умеет
type user struct {
	*oapi.ClientWithResponses
	http *http.Client
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(os.Getenv(key))
	if err != nil {
		return def
	}
	return d
}

// as входит учёткой один раз на весь прогон: api пускает не больше 10 входов в минуту с адреса
func as(t *testing.T, login string) *user {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if u, ok := users[login]; ok {
		return u
	}
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar}
	c, err := oapi.NewClientWithResponses(baseURL, oapi.WithHTTPClient(hc))
	require.NoError(t, err)
	res, err := c.LoginWithResponse(t.Context(), oapi.LoginRequest{Login: login, Password: "demo12345"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), "вход %s: %s", login, res.Body)
	users[login] = &user{c, hc}
	return users[login]
}

func post(t *testing.T, c *user, body oapi.NewApplication) *oapi.CreateApplicationResponse {
	t.Helper()
	key := &oapi.CreateApplicationParams{IdempotencyKey: uuid.NewString()}
	res, err := c.CreateApplicationWithResponse(t.Context(), key, body)
	require.NoError(t, err)
	return res
}

func create(t *testing.T, c *user, body oapi.NewApplication) oapi.ApplicationView {
	t.Helper()
	res := post(t, c, body)
	require.Equal(t, http.StatusCreated, res.StatusCode(), string(res.Body))
	return *res.JSON201
}

func upload(t *testing.T, c *user, app uuid.UUID, kind oapi.DocumentKind, file string) oapi.Document {
	t.Helper()
	data, err := os.ReadFile(docs + file)
	require.NoError(t, err)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("kind", string(kind)))
	// CreateFormFile ставит octet-stream, а спека принимает только pdf и картинки
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filepath.Base(file)+`"`)
	h.Set("Content-Type", http.DetectContentType(data))
	f, err := w.CreatePart(h)
	require.NoError(t, err)
	_, _ = f.Write(data)
	require.NoError(t, w.Close())

	res, err := c.UploadDocumentWithBodyWithResponse(t.Context(), app, w.FormDataContentType(), &buf)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode(), string(res.Body))
	return *res.JSON201
}

// confirm подтверждает итоги эталоном генератора, как будто клиент сверил их глазами.
// Эталон в рублях, контракт в копейках
func confirm(t *testing.T, c *user, doc oapi.Document, file string) {
	t.Helper()
	raw, err := os.ReadFile(docs + strings.TrimSuffix(file, filepath.Ext(file)) + ".truth.json")
	require.NoError(t, err)

	// кроме выписки подтверждается только отчётность
	var vals oapi.DocumentValues
	if doc.Kind == oapi.DocumentKindStatement {
		var s docgen.Statement
		require.NoError(t, json.Unmarshal(raw, &s))
		v := oapi.StatementValues{
			Inn:               s.INN,
			OpeningBalanceKop: int64(s.OpeningBalance),
			TurnoverInKop:     int64(s.TurnoverIn),
			TurnoverOutKop:    int64(s.TurnoverOut),
			ClosingBalanceKop: int64(s.ClosingBalance),
		}
		require.NoError(t, v.PeriodFrom.UnmarshalText([]byte(s.PeriodFrom)))
		require.NoError(t, v.PeriodTo.UnmarshalText([]byte(s.PeriodTo)))
		v.MonthlyIn = make([]struct {
			AmountKop int64  `json:"amount_kop"`
			Month     string `json:"month"`
		}, len(s.MonthlyIn))
		for i, m := range s.MonthlyIn {
			v.MonthlyIn[i].Month, v.MonthlyIn[i].AmountKop = m.Month, int64(m.Amount)
		}
		vals.Statement = &v
	} else {
		var f docgen.Financials
		require.NoError(t, json.Unmarshal(raw, &f))
		vals.Financials = &oapi.FinancialsValues{
			Inn:                 f.INN,
			Year:                f.Year,
			RevenueKop:          int64(f.Revenue),
			NetProfitKop:        int64(f.NetProfit),
			LiabilitiesLongKop:  int64(f.LiabilitiesLong),
			LiabilitiesShortKop: int64(f.LiabilitiesShort),
			BalanceTotalKop:     int64(f.BalanceTotal),
		}
	}
	res, err := c.ConfirmDocumentWithResponse(t.Context(), doc.Id, vals)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
}

func submit(t *testing.T, c *user, app uuid.UUID) {
	t.Helper()
	res, err := c.SubmitApplicationWithResponse(t.Context(), app)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
}

func card(t *testing.T, c *user, app uuid.UUID) oapi.ApplicationView {
	t.Helper()
	res, err := c.GetApplicationWithResponse(t.Context(), app)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	return *res.JSON200
}

// wait опрашивает карточку, пока заявка не встанет в want. Промежуточные статусы
// проходят за доли секунды и опросом ловятся не всегда, поэтому ждём только конечный
func wait(t *testing.T, c *user, app uuid.UUID, want oapi.ApplicationStatus) oapi.ApplicationView {
	t.Helper()
	return waitWithin(t, c, app, want, waitFor)
}

func waitWithin(
	t *testing.T,
	c *user,
	app uuid.UUID,
	want oapi.ApplicationStatus,
	d time.Duration,
) oapi.ApplicationView {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		v := card(t, c, app)
		if v.Status == want {
			return v
		}
		if time.Now().After(deadline) {
			require.Failf(t, "статус не дождались", "ждали %s, заявка %d в %s", want, v.Number, v.Status)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func codes(v oapi.ApplicationView) []string {
	out := make([]string, len(v.Reasons))
	for i, r := range v.Reasons {
		out[i] = string(r.Code)
	}
	return out
}

func history(t *testing.T, c *user, app uuid.UUID) []oapi.ApplicationEvent {
	t.Helper()
	res, err := c.ListApplicationEventsWithResponse(t.Context(), app)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	return *res.JSON200
}

// events подписывается на SSE и отдаёт статусы по заявке app в канал
func events(t *testing.T, c *user, app uuid.UUID) <-chan oapi.StreamEvent {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/events", http.NoBody)
	require.NoError(t, err)
	res, err := c.http.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)

	out := make(chan oapi.StreamEvent, 16)
	go func() {
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var ev oapi.StreamEvent
			if json.Unmarshal([]byte(data), &ev) == nil && ev.ApplicationId == app {
				out <- ev
			}
		}
	}()
	return out
}

// reasons причины последнего перехода из журнала: сбой источника клиенту в карточке
// не объясняем, он виден только банку
func reasons(evs []oapi.ApplicationEvent) []oapi.ReasonCode {
	for _, e := range slices.Backward(evs) {
		if e.Kind == oapi.ApplicationEventKindTransition && e.ReasonCodes != nil {
			return *e.ReasonCodes
		}
	}
	return nil
}

func hasEvent(evs []oapi.ApplicationEvent, kind oapi.ApplicationEventKind, to oapi.ApplicationStatus) bool {
	return slices.ContainsFunc(evs, func(e oapi.ApplicationEvent) bool {
		return e.Kind == kind && (to == "" || e.ToStatus != nil && *e.ToStatus == to)
	})
}
