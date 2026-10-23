package rules_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
)

const scoringPath = "../../rules/scoring.yaml"

func TestScoring(t *testing.T) {
	s, err := rules.Scoring(scoringPath)
	require.NoError(t, err)

	require.Len(t, s.Factors, 9)
	require.Equal(t, 16, s.Factors[0].Max)
	require.Nil(t, s.Factors[0].Bins[3].Lt)
	require.Equal(t, 6, s.Factors[8].Max)
	require.Empty(t, s.Factors[8].Groups[2].OKVED)
	require.Equal(t, "E", s.Grades[4].Grade)
	require.Equal(t, 3, s.TopReasons)
	require.True(t, s.Reasons["SC_AGE_MATURE"].Plus)
	require.False(t, s.Reasons["SC_DSR_HIGH"].Plus)
}

// кривой файл это правка настоящего, как в TestProductsBroken
func broken(t *testing.T, path, old, repl string) string {
	t.Helper()
	orig, err := os.ReadFile(path)
	require.NoError(t, err)
	src := strings.Replace(string(orig), old, repl, 1)
	require.NotEqual(t, string(orig), src, "замена не сработала")
	out := filepath.Join(t.TempDir(), filepath.Base(path))
	require.NoError(t, os.WriteFile(out, []byte(src), 0o600))
	return out
}

func TestScoringBroken(t *testing.T) {
	tests := []struct{ name, old, repl string }{
		{"опечатка в поле", "top_reasons: 3", "top_reason: 3"},
		{"неизвестный input", "input: turnover_cv", "input: turnover"},
		{"бины не по порядку", "{lt: 24, points: 6}", "{lt: 6, points: 6}"},
		{
			"граница у последнего бина",
			"{points: 16, reason: SC_AGE_MATURE}",
			"{lt: 100, points: 16, reason: SC_AGE_MATURE}",
		},
		{"бины и группы вместе", "groups: #", "bins: [{points: 0}]\n    groups: #"},
		{"последняя группа с кодами", "- {points: 3}", "- {okved: ['10'], points: 3}"},
		{"нет текста причины", "reason: SC_AGE_YOUNG}", "reason: SC_AGE_OLD}"},
		{"кривой sign", "SC_AGE_YOUNG: {sign: minus", "SC_AGE_YOUNG: {sign: neg"},
		{"сумма не 100", "{points: 16, reason: SC_AGE_MATURE}", "{points: 15, reason: SC_AGE_MATURE}"},
		{"классы не по убыванию", "{grade: B, min: 65", "{grade: B, min: 85"},
		{"последний класс не с нуля", "{grade: E, min: 0", "{grade: E, min: 10"},
		{"top_reasons ноль", "top_reasons: 3", "top_reasons: 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := rules.Scoring(broken(t, scoringPath, tt.old, tt.repl))
			require.Error(t, err)
		})
	}
}
