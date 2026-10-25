package mocks_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
)

const callback = "/api/auth/alfa-id/callback"

func TestAlfaIDPage(t *testing.T) {
	tests := []struct {
		name, redirect string
		code           int
	}{
		{"свой адрес", callback, http.StatusOK},
		{"чужой сайт", "https://evil.example/cb", http.StatusBadRequest},
		{"без схемы", "//evil.example/cb", http.StatusBadRequest},
		{"пусто", "", http.StatusBadRequest},
	}
	e := mocks.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := url.Values{"client_id": {"credit-cabinet"}, "redirect_uri": {tt.redirect}, "state": {"s1"}}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/alfa-id/authorize?"+q.Encode(), http.NoBody))
			require.Equal(t, tt.code, rec.Code, rec.Body.String())
			if tt.code == http.StatusOK {
				require.Contains(t, rec.Body.String(), "Соколова Анна Викторовна")
			}
		})
	}
}

func post(e http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestAlfaIDFlow(t *testing.T) {
	e := mocks.New()
	form := url.Values{"redirect_uri": {callback}, "state": {"s1"}, "login": {"coffee"}}

	rec := post(e, "/alfa-id/authorize", form)
	require.Equal(t, http.StatusFound, rec.Code)
	back, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, callback, back.Path)
	require.Equal(t, "s1", back.Query().Get("state"))
	code := back.Query().Get("code")
	require.NotEmpty(t, code)

	rec = post(e, "/alfa-id/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var p map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, "coffee", p["login"])
	require.Equal(t, "+79161000001", p["phone"])
	require.NotEmpty(t, p["sub"])

	rec = post(e, "/alfa-id/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}})
	require.Equal(t, http.StatusBadRequest, rec.Code, "код одноразовый")
}

func TestAlfaIDAuthorize(t *testing.T) {
	tests := []struct {
		name  string
		form  url.Values
		code  int
		query string // что уходит обратно в redirect_uri
	}{
		{
			"отказ", url.Values{"redirect_uri": {callback}, "state": {"s1"}, "deny": {"1"}},
			http.StatusFound, "error=access_denied&state=s1",
		},
		{
			"нет такого клиента",
			url.Values{"redirect_uri": {callback}, "state": {"s1"}, "login": {"nobody"}},
			http.StatusBadRequest,
			"",
		},
		{
			"чужой redirect_uri",
			url.Values{"redirect_uri": {"https://evil.example"}, "login": {"coffee"}},
			http.StatusBadRequest,
			"",
		},
	}
	e := mocks.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := post(e, "/alfa-id/authorize", tt.form)
			require.Equal(t, tt.code, rec.Code, rec.Body.String())
			if tt.query != "" {
				require.Equal(t, callback+"?"+tt.query, rec.Header().Get("Location"))
			}
		})
	}
}

func TestAlfaIDBadCode(t *testing.T) {
	rec := post(mocks.New(), "/alfa-id/token", url.Values{"grant_type": {"authorization_code"}, "code": {"junk"}})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
