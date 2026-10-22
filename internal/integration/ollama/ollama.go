// Package ollama: клиент локальной модели, которая раскладывает текст
// документа по полям. Ответ ограничен JSON-схемой вида документа, но всё равно
// недоверенный: разбираем строго, проверки сумм и дат дальше в domain.
package ollama

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/ollama/ollama/api"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// схемы уходят в format как есть, промпт системным сообщением
//
//go:embed schema/*.json prompt/*.txt
var files embed.FS

const (
	// num_ctx 8192 токенов, а русский текст это примерно 2-3 символа на токен
	maxText = 12_000
	numCtx  = 8192

	kopPerRub = 100
)

type Client struct {
	api     *api.Client
	model   string
	timeout time.Duration
}

func New(baseURL, model string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("ollama url: %w", err)
	}
	return &Client{api: api.NewClient(u, &http.Client{Timeout: timeout}), model: model, timeout: timeout}, nil
}

func (c *Client) Model() string { return c.model }

// Digest берём из списка моделей: show его не отдаёт.
func (c *Client) Digest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	list, err := c.api.List(ctx)
	if err != nil {
		return "", fmt.Errorf("ollama list: %w", err)
	}
	for i := range list.Models {
		if m := &list.Models[i]; m.Name == c.model {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("ollama: модели %s нет, нужен ollama pull", c.model)
}

// Extract отдаёт ответ модели как есть и поля из него в копейках. Ответ не по
// схеме это domain.ErrInvalid: при temperature 0 повтор вернёт то же самое.
func (c *Client) Extract(ctx context.Context, kind domain.DocKind, text string) ([]byte, domain.DocValues, error) {
	schema, err := files.ReadFile("schema/" + string(kind) + ".json")
	if err != nil {
		return nil, domain.DocValues{}, fmt.Errorf("%w: документы вида %q не распознаём", domain.ErrInvalid, kind)
	}
	prompt, err := files.ReadFile("prompt/" + string(kind) + ".txt")
	if err != nil {
		return nil, domain.DocValues{}, fmt.Errorf("prompt %s: %w", kind, err)
	}
	if r := []rune(text); len(r) > maxText {
		text = string(r[:maxText])
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	stream := false
	var raw []byte
	err = c.api.Chat(ctx, &api.ChatRequest{
		Model: c.model,
		Messages: []api.Message{
			{Role: "system", Content: string(prompt)},
			{Role: "user", Content: text},
		},
		Stream:  &stream,
		Format:  schema,
		Options: map[string]any{"temperature": 0, "num_ctx": numCtx},
	}, func(r api.ChatResponse) error {
		raw = []byte(r.Message.Content)
		return nil
	})
	if err != nil {
		return nil, domain.DocValues{}, fmt.Errorf("ollama %s: %w", c.model, err)
	}

	var v domain.DocValues
	switch kind {
	case domain.DocStatement:
		var s statement
		if err = strict(raw, &s); err == nil {
			v.Statement = s.domain()
		}
	case domain.DocFinancials:
		var f financials
		if err = strict(raw, &f); err == nil {
			v.Financials = f.domain()
		}
	}
	return raw, v, err
}

func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: ответ модели не по схеме: %w", domain.ErrInvalid, err)
	}
	return nil
}

// statement и financials это ответ модели: суммы в рублях, как в документе
type statement struct {
	INN            string  `json:"inn"`
	PeriodFrom     string  `json:"period_from"`
	PeriodTo       string  `json:"period_to"`
	OpeningBalance float64 `json:"opening_balance"`
	TurnoverIn     float64 `json:"turnover_in"`
	TurnoverOut    float64 `json:"turnover_out"`
	ClosingBalance float64 `json:"closing_balance"`
	MonthlyIn      []struct {
		Month  string  `json:"month"`
		Amount float64 `json:"amount"`
	} `json:"monthly_in"`
}

func (s *statement) domain() *domain.Statement {
	d := &domain.Statement{
		INN:            s.INN,
		PeriodFrom:     s.PeriodFrom,
		PeriodTo:       s.PeriodTo,
		OpeningBalance: kop(s.OpeningBalance),
		TurnoverIn:     kop(s.TurnoverIn),
		TurnoverOut:    kop(s.TurnoverOut),
		ClosingBalance: kop(s.ClosingBalance),
	}
	for _, m := range s.MonthlyIn {
		d.MonthlyIn = append(d.MonthlyIn, domain.MonthAmount{Month: m.Month, Amount: kop(m.Amount)})
	}
	return d
}

type financials struct {
	INN              string  `json:"inn"`
	Year             int     `json:"year"`
	Revenue          float64 `json:"revenue"`
	NetProfit        float64 `json:"net_profit"`
	LiabilitiesLong  float64 `json:"liabilities_long"`
	LiabilitiesShort float64 `json:"liabilities_short"`
	BalanceTotal     float64 `json:"balance_total"`
}

func (f *financials) domain() *domain.Financials {
	return &domain.Financials{
		INN:              f.INN,
		Year:             f.Year,
		Revenue:          kop(f.Revenue),
		NetProfit:        kop(f.NetProfit),
		LiabilitiesLong:  kop(f.LiabilitiesLong),
		LiabilitiesShort: kop(f.LiabilitiesShort),
		BalanceTotal:     kop(f.BalanceTotal),
	}
}

// float64 держит рубли с копейками точно до 2^53 копеек, выписке хватает
func kop(rub float64) int64 { return int64(math.Round(rub * kopPerRub)) }
