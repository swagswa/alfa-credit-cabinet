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
		{rbac.Anonymous, "/api/me", "GET", true},
		{"client", "/api/me", "GET", true},
		{"client", "/api/me", "POST", false},
		// вошедший не теряет того, что может аноним: иначе повторный вход
		// и health в браузере отдают 403
		{"client", "/api/login", "POST", true},
		{"client", "/api/health", "GET", true},
		{"manager", "/api/login", "POST", true},
		{"underwriter", "/api/health", "GET", true},
		{"manager", "/api/companies/0000100012", "GET", false},
		{"admin", "/api/me", "GET", true},
		{"client", "/api/applications", "POST", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41", "PATCH", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/cancel", "POST", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/submit", "POST", true},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/submit", "POST", false},
		{"manager", "/api/applications", "GET", true},
		{"underwriter", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41", "GET", true},
		{"admin", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/history", "GET", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/history", "GET", true},
		{"manager", "/api/applications", "POST", false},
		{"admin", "/api/admin/faults", "PUT", true},
		{"client", "/api/profile", "GET", true},
		{"client", "/api/notifications", "GET", true},
		{"client", "/api/notifications/42/read", "POST", true},
		{"client", "/api/notifications/read-all", "POST", true},
		{"manager", "/api/notifications", "GET", false},
		{"admin", "/api/notifications/read-all", "POST", false},
		{"manager", "/api/profile", "GET", false},
		{"admin", "/api/profile", "GET", false},
		{rbac.Anonymous, "/api/profile", "GET", false},
		{"manager", "/api/admin/faults", "GET", false},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41", "PATCH", false},
		{rbac.Anonymous, "/api/events", "GET", false},
		{"client", "/api/events", "GET", true},
		{"underwriter", "/api/events", "GET", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/documents", "POST", true},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/documents", "POST", false},
		{"underwriter", "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52", "GET", true},
		{"underwriter", "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52/file", "GET", true},
		{"client", "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52/confirm", "POST", true},
		{"underwriter", "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52/confirm", "POST", false},
		{rbac.Anonymous, "/api/documents/0192b6f5-1a20-7c4e-8d11-3e9f0a7b6c52/file", "GET", false},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/provide-info", "POST", true},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/provide-info", "POST", false},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/request-info", "POST", true},
		{"underwriter", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/request-info", "POST", true},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/request-info", "POST", false},
		{"underwriter", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/decision", "POST", true},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/decision", "POST", false},
		{"client", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/decision", "POST", false},
		{"manager", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/retry-checks", "POST", true},
		{"underwriter", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/retry-checks", "POST", false},
		{"admin", "/api/applications/0192b6f4-7c1e-7a3d-9b2a-5f0c1d2e3a41/retry-checks", "POST", true},
		{"client", "/api/feed", "GET", true},
		{"manager", "/api/feed", "GET", false},
		{"admin", "/api/feed", "GET", false},
	}
	for _, tt := range tests {
		t.Run(tt.role+" "+tt.method+" "+tt.path, func(t *testing.T) {
			ok, err := e.Allowed(tt.role, tt.path, tt.method)
			require.NoError(t, err)
			require.Equal(t, tt.ok, ok)
		})
	}
}
