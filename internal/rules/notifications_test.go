package rules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
)

const noticesPath = "../../rules/notifications.yaml"

func TestNotices(t *testing.T) {
	r, err := rules.Notices(noticesPath)
	require.NoError(t, err)
	require.Contains(t, r.Statuses, domain.StatusIssued)
	require.NotContains(t, r.Statuses, domain.StatusChecking, "на внутренние шаги банка клиента не дёргаем")
	require.Contains(t, r.SigningCode.Text, "{code}")
}

func TestNoticesBroken(t *testing.T) {
	orig, err := os.ReadFile(noticesPath)
	require.NoError(t, err)

	tests := []struct{ name, old, repl string }{
		{"опечатка в поле", "signing_code: {title", "signing_kode: {title"},
		{"неизвестный статус", "  rejected: {", "  rejectd: {"},
		{"фраза без текста", "text: 'Причины и что можно сделать, в карточке заявки'", "text: ''"},
		{"код без подстановки", "Код {code} для", "Код для"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(string(orig), tt.old, tt.repl, 1)
			require.NotEqual(t, string(orig), src, "замена не сработала")
			path := filepath.Join(t.TempDir(), "notifications.yaml")
			require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

			_, err := rules.Notices(path)
			require.Error(t, err)
		})
	}
}
