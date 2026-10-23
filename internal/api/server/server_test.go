package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/handler"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/middleware"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
	"github.com/swagswa/alfa-credit-cabinet/internal/api/server"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/token"
)

const (
	recommend        = "/api/products/recommendation?goal=cash_gap&amount_kop=100000000"
	recommendLeasing = "/api/products/recommendation?goal=leasing&amount_kop=100000000&term_months=2"
)

type egrulDown struct{}

// одна учётка клиента: как в сиде, coffee с паролем demo12345
type fakeUsers struct {
	user domain.User
	hash []byte
}

func (f *fakeUsers) Credentials(_ context.Context, login string) (domain.User, []byte, error) {
	if login != f.user.Login {
		return domain.User{}, nil, domain.ErrNotFound
	}
	return f.user, f.hash, nil
}

func (f *fakeUsers) ByID(context.Context, uuid.UUID) (domain.User, error) { return f.user, nil }

func (f *fakeUsers) Memberships(context.Context, uuid.UUID) ([]domain.Membership, error) {
	return nil, nil
}

type fakeSessions struct {
	mu    sync.Mutex
	saved map[string]domain.Session
	hits  map[string]int64
}

func (f *fakeSessions) Save(_ context.Context, s domain.Session, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved[s.ID] = s
	return nil
}

func (f *fakeSessions) Find(_ context.Context, id string) (domain.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.saved[id]
	if !ok {
		return domain.Session{}, domain.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.saved, id)
	return nil
}

func (f *fakeSessions) Hit(_ context.Context, key string, _ time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits[key]++
	return f.hits[key], nil
}

func newAuth(t *testing.T) usecase.AuthUseCase {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("demo12345"), bcrypt.MinCost)
	require.NoError(t, err)
	users := &fakeUsers{user: domain.User{ID: uuid.New(), Login: "coffee", Role: domain.RoleClient}, hash: hash}
	sessions := &fakeSessions{saved: map[string]domain.Session{}, hits: map[string]int64{}}
	limit := usecase.LoginLimit{Attempts: 3, Window: time.Minute}
	return usecase.NewAuth(users, sessions, token.New("test-secret", time.Hour), limit)
}

func newServer(t *testing.T, auth usecase.AuthUseCase, dbErr error) http.Handler {
	t.Helper()
	enf, err := rbac.New()
	require.NoError(t, err)
	catalog, err := rules.Products("../../../rules/products.yaml")
	require.NoError(t, err)
	health := usecase.NewHealth(map[string]usecase.Check{
		"db":       func(context.Context) error { return dbErr },
		"temporal": func(context.Context) error { return nil },
	})
	e, err := server.New(server.Deps{
		Log: zap.NewNop(),
		Handler: handler.New(handler.Deps{
			Health:     health,
			Products:   usecase.NewProduct(catalog),
			Companies:  usecase.NewCompany(egrulDown{}),
			Auth:       auth,
			Documents:  &fakeDocs{},
			Issue:      fakeIssue{},
			SessionTTL: time.Hour,
		}),
		Auth: auth,
		RBAC: enf,
	})
	require.NoError(t, err)
	return e
}

func call(t *testing.T, h http.Handler, method, path, body, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != "" {
		req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: session})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (egrulDown) Company(context.Context, string) (domain.Company, error) {
	return domain.Company{}, domain.ErrUnavailable
}

func TestServer(t *testing.T) {
	auth := newAuth(t)
	_, client, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)

	tests := []struct {
		name    string
		dbErr   error
		method  string
		path    string
		session string
		status  int
	}{
		{name: "health без сессии", method: "GET", path: "/api/health", status: 200},
		{name: "health при упавшей базе", dbErr: errors.New("down"), method: "GET", path: "/api/health", status: 503},
		{name: "закрытая операция без сессии", method: "GET", path: "/api/me", status: 401},
		{name: "битая сессия как её отсутствие", method: "GET", path: "/api/me", session: "junk", status: 401},
		{name: "текущий пользователь", method: "GET", path: "/api/me", session: client, status: 200},
		{name: "health с сессией клиента", method: "GET", path: "/api/health", session: client, status: 200},
		{
			name:    "роли не положено",
			method:  "POST",
			path:    "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/decision",
			session: client,
			status:  403,
		},
		{name: "продукты", method: "GET", path: "/api/products", session: client, status: 200},
		{name: "подбор", method: "GET", path: recommend + "&term_months=2", session: client, status: 200},
		{name: "подбор без срока", method: "GET", path: recommend, session: client, status: 400},
		{name: "подбор с чужой целью", method: "GET", path: recommendLeasing, session: client, status: 400},
		{name: "инн короче 10", method: "GET", path: "/api/companies/000010001", session: client, status: 400},
		{name: "инн с неверной контрольной", method: "GET", path: "/api/companies/0000100013", session: client,
			status: 400},
		{name: "источник лежит", method: "GET", path: "/api/companies/0000500010", session: client, status: 503},
		{name: "события без сессии", method: "GET", path: "/api/events", status: 401},
		// валидатор на /events пропущен, параметр проверяет хендлер
		{name: "события с битым id", method: "GET", path: "/api/events?application_id=1", session: client, status: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(t, newServer(t, auth, tt.dbErr), tt.method, tt.path, "", tt.session)

			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status >= 400 && tt.status != 503 {
				require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestLoginLogout(t *testing.T) {
	h := newServer(t, newAuth(t), nil)

	rec := call(t, h, "POST", "/api/login", `{"login":"coffee","password":"wrong"}`, "")
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	require.Empty(t, rec.Result().Cookies())

	rec = call(t, h, "POST", "/api/login", `{"login":"coffee","password":"demo12345"}`, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	ck := rec.Result().Cookies()
	require.Len(t, ck, 1)
	require.True(t, ck[0].HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, ck[0].SameSite)
	session := ck[0].Value

	require.Equal(t, http.StatusOK, call(t, h, "GET", "/api/me", "", session).Code)

	rec = call(t, h, "POST", "/api/logout", "", session)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Negative(t, rec.Result().Cookies()[0].MaxAge, "cookie стирается")

	// токен по подписи ещё жив, но сессии на сервере больше нет
	require.Equal(t, http.StatusUnauthorized, call(t, h, "GET", "/api/me", "", session).Code)
}

func TestLoginLimit(t *testing.T) {
	h := newServer(t, newAuth(t), nil)
	for range 3 {
		require.Equal(
			t,
			http.StatusUnauthorized,
			call(t, h, "POST", "/api/login", `{"login":"coffee","password":"x"}`, "").Code,
		)
	}
	rec := call(t, h, "POST", "/api/login", `{"login":"coffee","password":"demo12345"}`, "")
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "too_many_requests")
}

type fakeDocs struct {
	usecase.DocumentUseCase
}

func (f *fakeDocs) Upload(
	_ context.Context, _ domain.Actor, app uuid.UUID, kind domain.DocKind, name string, file []byte,
) (domain.Document, error) {
	return domain.Document{ID: uuid.New(), ApplicationID: app, Kind: kind, Status: domain.DocSkipped,
		FileName: name, ContentType: "application/pdf", Size: len(file), SHA256: make([]byte, 32)}, nil
}

func (f *fakeDocs) Confirm(context.Context, domain.Actor, uuid.UUID, domain.DocValues) (domain.Document, error) {
	return domain.Document{}, domain.FieldErrors{{Field: "inn", Message: "ИНН в документе другой"}}
}

func multipartBody(t *testing.T, kind string, file []byte) (body *bytes.Buffer, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("kind", kind))
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="vypiska.pdf"`)
	h.Set("Content-Type", "application/pdf")
	part, err := w.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write(file)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func TestDocumentsHTTP(t *testing.T) {
	auth := newAuth(t)
	_, client, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)
	const upload = "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/documents"
	pdf := []byte("%PDF-1.4 выписка")

	tests := []struct {
		name   string
		kind   string
		file   []byte
		status int
	}{
		{"выписка", "statement", pdf, http.StatusCreated},
		{"неизвестный вид", "selfie", pdf, http.StatusBadRequest},
		{"больше лимита тела", "statement", make([]byte, 11<<20), http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, ct := multipartBody(t, tt.kind, tt.file)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, upload, body)
			req.Header.Set("Content-Type", ct)
			req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: client})
			rec := httptest.NewRecorder()
			newServer(t, auth, nil).ServeHTTP(rec, req)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			if tt.status == http.StatusCreated {
				require.Equal(t, `"vypiska.pdf"`, field(t, rec.Body.Bytes(), "file_name"))
				require.Equal(t, `"statement"`, field(t, rec.Body.Bytes(), "kind"))
				require.Equal(t, strconv.Itoa(len(pdf)), field(t, rec.Body.Bytes(), "size_bytes"))
			}
		})
	}

	// итоги не сошлись: 422 с ошибками по полям, фронт подсветит их в форме
	rec := call(t, newServer(t, auth, nil), "POST", "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52/confirm",
		`{"financials":{"inn":"0000100012","year":2025,"revenue_kop":1,"net_profit_kop":0,`+
			`"liabilities_long_kop":0,"liabilities_short_kop":0,"balance_total_kop":1}}`, client)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.JSONEq(t, `[{"field":"inn","message":"ИНН в документе другой"}]`,
		field(t, rec.Body.Bytes(), "errors"))
}

func field(t *testing.T, b []byte, key string) string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &m))
	return string(m[key])
}

// fakeIssue: верный код 482913, всё остальное мимо
type fakeIssue struct {
	usecase.IssueUseCase
}

func (fakeIssue) RequestCode(context.Context, domain.Actor, uuid.UUID) (string, domain.SigningCode, error) {
	at := time.Date(2026, time.October, 24, 10, 5, 0, 0, time.UTC)
	return "482913", domain.SigningCode{AttemptsLeft: 3, ExpiresAt: at}, nil
}

func (fakeIssue) Sign(_ context.Context, _ domain.Actor, _ uuid.UUID, code string) error {
	if code == "482913" {
		return nil
	}
	return domain.FieldErrors{{Field: "code", Message: "неверный код, осталось попыток: 2"}}
}

func TestSigningHTTP(t *testing.T) {
	auth := newAuth(t)
	_, client, err := auth.Login(t.Context(), "coffee", "demo12345", "10.0.0.1")
	require.NoError(t, err)
	const app = "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41"
	h := newServer(t, auth, nil)

	rec := call(t, h, "POST", app+"/signing-code", "", client)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, `{"code":"482913","expires_at":"2026-10-24T10:05:00Z","attempts_left":3}`, rec.Body.String())

	rec = call(t, h, "POST", app+"/sign", `{"code":"000000"}`, client)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	require.JSONEq(t, `[{"field":"code","message":"неверный код, осталось попыток: 2"}]`,
		field(t, rec.Body.Bytes(), "errors"))
	require.NotContains(t, rec.Body.String(), "482913")

	rec = call(t, h, "POST", app+"/sign", `{"code":"12"}`, client)
	require.Equal(t, http.StatusBadRequest, rec.Code, "код не из 6 цифр режет валидатор")
}

// виды событий журнала из CHECK в схеме: история отдаёт их как есть, контракт должен их знать
func TestEventKinds(t *testing.T) {
	for _, k := range []string{"transition", "escalation", "override", "signing"} {
		require.True(t, oapi.ApplicationEventKind(k).Valid(), k)
	}
}
