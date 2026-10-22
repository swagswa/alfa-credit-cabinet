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

const productsPath = "../../rules/products.yaml"

func TestProducts(t *testing.T) {
	c, err := rules.Products(productsPath)
	require.NoError(t, err)

	require.Equal(t, 1400, c.KeyRateBP)
	require.Equal(t, int64(1_000_000), c.AmountStepKop)
	require.Len(t, c.Products, 3)

	wc, ok := c.Product(domain.WorkingCapital)
	require.True(t, ok)
	require.Equal(t, int64(30_000_000), wc.AmountMinKop)
	require.Equal(t, int64(1_000_000_000), wc.AmountMaxKop)
	require.Equal(t, 1900, wc.BaseRateBP)
	require.Equal(t, int64(300_000_000), wc.GuarantorOverKop)

	g, ok := c.Goal(domain.GoalCashGap)
	require.True(t, ok)
	require.Equal(t, &domain.Condition{Input: domain.InputTermMonths, Lt: 3}, g.Picks[0].When)
	require.Equal(t, domain.Overdraft, g.Picks[0].Product)
}

// кривой файл получаем правкой настоящего, чтобы тест не разъезжался с ним
func TestProductsBroken(t *testing.T) {
	orig, err := os.ReadFile(productsPath)
	require.NoError(t, err)

	tests := []struct{ name, old, repl string }{
		{"опечатка в поле", "amount_min_rub: 300000", "amount_mn_rub: 300000"},
		{"мин больше макс", "amount_min_rub: 300000", "amount_min_rub: 30000000"},
		{"неизвестное погашение", "repayment: auto_debit", "repayment: monthly"},
		{"неизвестный продукт в подборе", "{product: working_capital}", "{product: leasing}"},
		{"неизвестный вход условия", "input: amount_rub", "input: revenue"},
		{"нет строки без условия", "- {product: working_capital, alternative: overdraft}\n", ""},
		{"нулевой шаг суммы", "amount_step_rub: 10000", "amount_step_rub: 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(string(orig), tt.old, tt.repl, 1)
			require.NotEqual(t, string(orig), src, "замена не сработала")
			path := filepath.Join(t.TempDir(), "products.yaml")
			require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

			_, err := rules.Products(path)
			require.Error(t, err)
		})
	}
}
