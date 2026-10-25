//go:build eval

package usecase_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ollama"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

// точность подбора по тексту на testdata/needs.yaml: модель, разбор суммы и рамки продуктов.
// Таблицу из вывода руками переносим в docs/ai-eval.md, ниже отметки
func TestNeedsEval(t *testing.T) {
	cfg, err := config.Load[config.Recognition]()
	require.NoError(t, err)
	llm, err := ollama.New(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaTimeout)
	require.NoError(t, err)
	c, err := rules.Products("../../rules/products.yaml")
	require.NoError(t, err)
	uc := usecase.NewProduct(c, llm)

	raw, err := os.ReadFile("../../testdata/needs.yaml")
	require.NoError(t, err)
	var cases []struct {
		Text       string      `yaml:"text"`
		Goal       domain.Goal `yaml:"goal"`
		AmountRub  int64       `yaml:"amount_rub"`
		TermMonths int         `yaml:"term_months"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &cases))

	fields := []string{"goal", "amount", "term_months"}
	hits := make([]int, len(fields))
	var report, misses strings.Builder
	for _, tc := range cases {
		n, err := uc.Need(t.Context(), tc.Text)
		require.NoError(t, err, tc.Text)
		ok := []bool{n.Goal == tc.Goal, n.AmountKop == tc.AmountRub*domain.KopInRub, n.TermMonths == tc.TermMonths}
		for i := range ok {
			if ok[i] {
				hits[i]++
			}
		}
		if !ok[0] || !ok[1] || !ok[2] {
			fmt.Fprintf(&misses, "| %s | %s, %d, %d |\n", tc.Text, n.Goal, n.AmountKop/domain.KopInRub, n.TermMonths)
		}
	}
	report.WriteString("\n| Поле | Верно |\n|---|---|\n")
	for i, f := range fields {
		share := 100 * float64(hits[i]) / float64(len(cases))
		fmt.Fprintf(&report, "| %s | %d из %d, %.1f%% |\n", f, hits[i], len(cases), share)
	}
	t.Log(report.String() + "\nОшибки: фраза | что получили\n" + misses.String())
	for i, f := range fields {
		require.GreaterOrEqual(t, float64(hits[i])/float64(len(cases)), 0.9, "цель 90%%: %s", f)
	}
}
