package rbac_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/rbac"
)

func TestAllowed(t *testing.T) {
	e, err := rbac.New()
	require.NoError(t, err)

	tests := []struct {
		role, path, method string
		ok                 bool
	}{
		{rbac.Anonymous, "/api/health", "GET", true},
		{rbac.Anonymous, "/api/samples/0000100012", "GET", true},
		{rbac.Anonymous, "/api/me", "GET", false},
		{"client", "/api/me", "GET", true},
		{"client", "/api/me", "POST", false},
		// вошедший не теряет того, что может аноним: иначе повторный вход,
		// health в браузере и «Взять пример выписки» отдают 403
		{"client", "/api/login", "POST", true},
		{"client", "/api/health", "GET", true},
		{"client", "/api/samples/0000100012", "GET", true},
		{"manager", "/api/login", "POST", true},
		{"underwriter", "/api/health", "GET", true},
		{"admin", "/api/samples/0000100012", "GET", true},
		{"manager", "/api/companies/0000100012", "GET", false},
		{"admin", "/api/me", "GET", true},
		{"client", "/api/applications", "POST", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41", "PATCH", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/cancel", "POST", true},
		// очередь банка откроется отдельно, пока сотрудник заявок не видит
		{"manager", "/api/applications", "GET", false},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41", "PATCH", false},
	}
	for _, tt := range tests {
		t.Run(tt.role+" "+tt.method+" "+tt.path, func(t *testing.T) {
			ok, err := e.Allowed(tt.role, tt.path, tt.method)
			require.NoError(t, err)
			require.Equal(t, tt.ok, ok)
		})
	}
}
