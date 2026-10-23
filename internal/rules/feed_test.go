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

const feedPath = "../../rules/feed.yaml"

// профили как у демо-компаний: ОКВЭД из сида, продукты из фикстур мока банка
func TestFeedTips(t *testing.T) {
	r, err := rules.Feed(feedPath)
	require.NoError(t, err)

	coffee := domain.Profile{Okved: "56.30", LastStatus: domain.NoApplication}
	logistics := domain.Profile{Okved: "49.41", LastStatus: domain.NoApplication}
	checked := func(p domain.Profile, status domain.Status, turnover int64, products ...string) domain.Profile {
		p.LastStatus, p.Bank, p.Products, p.TurnoverMonthRub = string(status), true, products, turnover
		return p
	}
	tests := []struct {
		name string
		p    domain.Profile
		want []string
	}{
		{"кофейня до заявки", coffee, []string{"first_application", "catering_support"}},
		{"логистика до заявки", logistics, []string{"first_application", "transport_equipment"}},
		{
			"кофейня после проверки",
			checked(coffee, domain.StatusApproved, 2_000_000, "account", "acquiring"),
			[]string{"catering_overdraft", "catering_support"},
		},
		{
			"логистика после проверки",
			checked(logistics, domain.StatusScoring, 1_500_000, "account"),
			[]string{"transport_equipment", "transport_overdraft"},
		},
		{
			"логистика с овердрафтом и малым оборотом",
			checked(logistics, domain.StatusIssued, 200_000, "account", "overdraft"),
			[]string{"transport_equipment", "low_turnover"},
		},
		{"не клиент банка", checked(coffee, domain.StatusRejected, 0), []string{"catering_support", "no_account"}},
		{"стройка без проверки", domain.Profile{Okved: "41.20", LastStatus: "need_info"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, tip := range r.Pick(&tt.p) {
				got = append(got, tip.Code)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFeedEvent(t *testing.T) {
	r, err := rules.Feed(feedPath)
	require.NoError(t, err)

	it, ok := r.Event(&domain.StatusEvent{ID: 42, Number: 7, Status: domain.StatusApproved})
	require.True(t, ok)
	require.Equal(t, "ev-42", it.ID)
	require.Equal(t, "Заявка №7 одобрена", it.Title)

	_, ok = r.Event(&domain.StatusEvent{Status: domain.StatusDraft})
	require.False(t, ok)
}

func TestFeedBroken(t *testing.T) {
	orig, err := os.ReadFile(feedPath)
	require.NoError(t, err)

	tests := []struct{ name, old, repl string }{
		{"опечатка в поле", "code: no_account", "kod: no_account"},
		{"неизвестный input", "input: okved_class, eq: '49'}", "input: okved, eq: '49'}"},
		{"строковый оператор у оборота", "turnover_month_rub, lt: 300000", "turnover_month_rub, eq: '300000'"},
		{"числовой оператор у ОКВЭД", "okved_class, eq: '49'}", "okved_class, gt: 49}"},
		{"два оператора", "{input: application, eq: none}", "{input: application, eq: none, ne: draft}"},
		{"неизвестный статус", "{input: application, eq: none}", "{input: application, eq: done}"},
		{"нет фразы на статус", "  expired: {", "  expird: {"},
		{"подсказка без текста", "title: Счёт в банке ускорит проверку", "title: ''"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := strings.Replace(string(orig), tt.old, tt.repl, 1)
			require.NotEqual(t, string(orig), src, "замена не сработала")
			path := filepath.Join(t.TempDir(), "feed.yaml")
			require.NoError(t, os.WriteFile(path, []byte(src), 0o600))

			_, err := rules.Feed(path)
			require.Error(t, err)
		})
	}
}
