package ollama_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ollama"
)

const model = "qwen3:4b-instruct-2507-q4_K_M"

// fake отвечает на /api/chat заданным content, как Ollama без стрима
func fake(t *testing.T, content string) *ollama.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req api.ChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Equal(t, model, req.Model)
		require.NotEmpty(t, req.Format)
		require.EqualValues(t, 0, req.Options["temperature"])
		_ = json.NewEncoder(w).Encode(api.ChatResponse{
			Model:   model,
			Message: api.Message{Role: "assistant", Content: content},
			Done:    true,
		})
	}))
	t.Cleanup(srv.Close)
	c, err := ollama.New(srv.URL, model, time.Second)
	require.NoError(t, err)
	return c
}

func TestExtract(t *testing.T) {
	tests := []struct {
		name    string
		kind    domain.DocKind
		content string
		want    domain.DocValues
		wantErr error
	}{
		{
			name: "выписка, суммы как в документе",
			kind: domain.DocStatement,
			content: `{"inn":"0000100012","period_from":"01.07.2026","period_to":"2026-09-30",
				"opening_balance":"1 133 359,35","turnover_in":"0.1","turnover_out":"0","closing_balance":"-5,5",
				"monthly_in":[{"year":2026,"month":7,"amount":"1234567.89"}]}`,
			want: domain.DocValues{Statement: &domain.Statement{
				INN: "0000100012", PeriodFrom: "2026-07-01", PeriodTo: "2026-09-30",
				OpeningBalance: 113335935, TurnoverIn: 10, ClosingBalance: -550,
				MonthlyIn: []domain.MonthAmount{{Month: "2026-07", Amount: 123456789}},
			}},
		},
		{
			name: "отчётность, убыток в скобках",
			kind: domain.DocFinancials,
			content: `{"inn":"0000100012","year":2025,"revenue":"14 399 999","net_profit":"(1 401 903)",
				"liabilities_long":"0","liabilities_short":"1 958 361","balance_total":"2 831 636"}`,
			want: domain.DocValues{Financials: &domain.Financials{
				INN: "0000100012", Year: 2025, Revenue: 1439999900, NetProfit: -140190300,
				LiabilitiesShort: 195836100, BalanceTotal: 283163600,
			}},
		},
		{
			name:    "лишнее поле",
			kind:    domain.DocFinancials,
			content: `{"inn":"0000100012","year":2025,"comment":"выручка примерно"}`,
			wantErr: domain.ErrInvalid,
		},
		{
			name:    "сумма числом",
			kind:    domain.DocStatement,
			content: `{"inn":"0000100012","turnover_in":1234}`,
			wantErr: domain.ErrInvalid,
		},
		{
			name:    "сумма не разбирается",
			kind:    domain.DocFinancials,
			content: `{"inn":"0000100012","year":2025,"revenue":"14 399 999,123"}`,
			wantErr: domain.ErrInvalid,
		},
		{name: "не json", kind: domain.DocStatement, content: "Выписка за период", wantErr: domain.ErrInvalid},
		{name: "паспорт не распознаём", kind: "passport", content: "{}", wantErr: domain.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, v, err := fake(t, tt.content).Extract(t.Context(), tt.kind, "текст документа")
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.want, v)
			if err == nil {
				require.JSONEq(t, tt.content, string(raw))
			}
		})
	}
}
