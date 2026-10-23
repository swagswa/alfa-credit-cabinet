package mocks_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/mocks"
)

func TestSample(t *testing.T) {
	tests := []struct {
		url  string
		code int
		ct   string
	}{
		{"/api/samples/0000300011?kind=statement", http.StatusOK, "application/pdf"},
		{"/api/samples/0000300011?kind=statement&format=c", http.StatusOK, "image/png"},
		{"/api/samples/0000300011?kind=financials&format=a", http.StatusOK, "application/pdf"},
		{"/api/samples/000020001010?kind=financials", http.StatusBadRequest, ""},
		{"/api/samples/0000300011?kind=passport", http.StatusBadRequest, ""},
	}
	e := mocks.New()
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.url, http.NoBody))
			require.Equal(t, tt.code, rec.Code, rec.Body.String())
			if tt.ct != "" {
				require.Equal(t, tt.ct, http.DetectContentType(rec.Body.Bytes()))
			}
		})
	}
}
