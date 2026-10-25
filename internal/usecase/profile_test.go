package usecase_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

func TestProfile(t *testing.T) {
	coffee, other := uuid.New(), uuid.New()
	bank := `{"is_client": true, "account_opened_at": "2024-10-15", "products": ["account", "acquiring"],
		"account": "40702810000000100012", "turnover_12m": [{"credit": 1500000}, {"credit": 2500000}]}`

	tests := []struct {
		name     string
		payload  string
		err      error
		bank     *domain.BankAccount
		bankDown bool
	}{
		{name: "клиент банка", payload: bank, bank: &domain.BankAccount{
			Number: "40702810000000100012", Since: time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC),
			TurnoverKop: 400000000, Products: []string{"account", "acquiring"},
		}},
		{name: "счёта в банке нет", err: fmt.Errorf("internal: %w", domain.ErrNotFound)},
		{name: "мок банка упал", err: fmt.Errorf("internal: %w", domain.ErrUnavailable), bankDown: true},
		{name: "не наш клиент", payload: `{"is_client": false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUsers{
				user:      domain.User{ID: uuid.New(), FullName: "Соколова Анна Викторовна", Phone: "+79000000001"},
				companies: []domain.ProfileCompany{{ID: coffee, INN: "0000100012"}},
				loans:     []domain.Loan{{Number: 1, CompanyID: coffee}, {Number: 2, CompanyID: other}},
			}
			src := &fakeSources{payload: json.RawMessage(tt.payload), err: tt.err}

			p, err := usecase.NewProfile(users, src).Profile(t.Context(), users.user.ID)
			require.NoError(t, err)
			require.Equal(t, users.user, p.User)
			require.Equal(t, "internal 0000100012 "+uuid.Nil.String(), src.called)
			require.Len(t, p.Companies, 1)
			c := p.Companies[0]
			require.Equal(t, tt.bank, c.Bank)
			require.Equal(t, tt.bankDown, c.BankDown)
			require.Equal(t, []domain.Loan{{Number: 1, CompanyID: coffee}}, c.Loans, "кредиты по своей компании")
		})
	}

	t.Run("без компаний", func(t *testing.T) {
		src := &fakeSources{}
		p, err := usecase.NewProfile(&fakeUsers{}, src).Profile(t.Context(), uuid.New())
		require.NoError(t, err)
		require.Empty(t, p.Companies)
		require.Empty(t, src.called, "в банк без компании не ходим")
	})
}
