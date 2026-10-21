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
	}
	for _, tt := range tests {
		t.Run(tt.role+" "+tt.method+" "+tt.path, func(t *testing.T) {
			ok, err := e.Allowed(tt.role, tt.path, tt.method)
			require.NoError(t, err)
			require.Equal(t, tt.ok, ok)
		})
	}
}
