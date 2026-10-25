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
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

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
	return &Client{
		api: api.NewClient(
			u,
			&http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)},
		),
		model:   model,
		timeout: timeout,
	}, nil
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
	if _, err := files.ReadFile("schema/" + string(kind) + ".json"); err != nil {
		return nil, domain.DocValues{}, fmt.Errorf("%w: документы вида %q не распознаём", domain.ErrInvalid, kind)
	}
	if r := []rune(text); len(r) > maxText {
		text = string(r[:maxText])
	}
	raw, err := c.chat(ctx, string(kind), text)
	if err != nil {
		return nil, domain.DocValues{}, err
	}

	var v domain.DocValues
	// сюда приходят только распознаваемые виды
	//exhaustive:ignore
	switch kind {
	case domain.DocStatement:
		var s statement
		if err = strict(raw, &s); err == nil {
			v.Statement, err = s.domain()
		}
	case domain.DocFinancials:
		var f financials
		if err = strict(raw, &f); err == nil {
			v.Financials, err = f.domain()
		}
	}
	if err != nil {
		return raw, domain.DocValues{}, err
	}
	return raw, v, nil
}

// chat: системный промпт и схема по имени из prompt/ и schema/, текст сообщением клиента
func (c *Client) chat(ctx context.Context, name, text string) ([]byte, error) {
	schema, err := files.ReadFile("schema/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("schema %s: %w", name, err)
	}
	prompt, err := files.ReadFile("prompt/" + name + ".txt")
	if err != nil {
		return nil, fmt.Errorf("prompt %s: %w", name, err)
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
		return nil, fmt.Errorf("ollama %s: %w", c.model, err)
	}
	return raw, nil
}

// Need достаёт цель, сумму и срок из слов клиента. Сумму модель переписывает как
// сказали («2 млн»), в копейки переводим сами. Цель не из списка это «другое».
func (c *Client) Need(ctx context.Context, text string) (domain.Need, error) {
	raw, err := c.chat(ctx, "need", text)
	if err != nil {
		return domain.Need{}, err
	}
	var n struct {
		Goal       domain.Goal `json:"goal"`
		Amount     string      `json:"amount"`
		TermMonths int         `json:"term_months"`
	}
	if err := strict(raw, &n); err != nil {
		return domain.Need{}, err
	}
	if !slices.Contains(domain.Goals, n.Goal) {
		n.Goal = domain.GoalOther
	}
	return domain.Need{Goal: n.Goal, AmountKop: spoken(n.Amount), TermMonths: max(n.TermMonths, 0)}, nil
}

var (
	// «3 500 000» и «3 500 000»: пробелы внутри числа
	digitGap = regexp.MustCompile(`(\d)[\s\x{00a0}]+(\d)`)
	number   = regexp.MustCompile(`\d+(?:[.,]\d+)?`)
)

// spoken: «1,5 миллиона», «800 тыс. рублей», «3 500 000 ₽» в копейки, 0 если числа нет
func spoken(s string) int64 {
	s = strings.ToLower(digitGap.ReplaceAllString(digitGap.ReplaceAllString(s, "$1$2"), "$1$2"))
	loc := number.FindStringIndex(s)
	if loc == nil {
		return 0
	}
	v, err := strconv.ParseFloat(strings.Replace(s[loc[0]:loc[1]], ",", ".", 1), 64)
	if err != nil {
		return 0
	}
	unit := s[loc[1]:]
	switch {
	case strings.Contains(unit, "млрд") || strings.Contains(unit, "миллиард"):
		v *= 1e9
	case strings.Contains(unit, "млн") || strings.Contains(unit, "миллион") || strings.Contains(unit, "лям"):
		v *= 1e6
	case strings.Contains(unit, "тыс") || strings.Contains(unit, "т.р"):
		v *= 1e3
	}
	return int64(math.Round(v * domain.KopInRub))
}

func strict(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: ответ модели не по схеме: %w", domain.ErrInvalid, err)
	}
	return nil
}

// statement и financials это ответ модели. Суммы строкой, как в документе:
// переписать «41 999 999,00» модели проще, чем перевести в число, на
// переводе 4B путает разряды. Число из строки делаем сами.
type statement struct {
	INN            string `json:"inn"`
	PeriodFrom     string `json:"period_from"`
	PeriodTo       string `json:"period_to"`
	OpeningBalance string `json:"opening_balance"`
	TurnoverIn     string `json:"turnover_in"`
	TurnoverOut    string `json:"turnover_out"`
	ClosingBalance string `json:"closing_balance"`
	MonthlyIn      []struct {
		// год и месяц числами: «10.2025» в «2025-10» модель переставляет с ошибками
		Year   int    `json:"year"`
		Month  int    `json:"month"`
		Amount string `json:"amount"`
	} `json:"monthly_in"`
}

func (s *statement) domain() (*domain.Statement, error) {
	var p parser
	d := &domain.Statement{
		INN:            s.INN,
		PeriodFrom:     date(s.PeriodFrom),
		PeriodTo:       date(s.PeriodTo),
		OpeningBalance: p.kop("opening_balance", s.OpeningBalance),
		TurnoverIn:     p.kop("turnover_in", s.TurnoverIn),
		TurnoverOut:    p.kop("turnover_out", s.TurnoverOut),
		ClosingBalance: p.kop("closing_balance", s.ClosingBalance),
	}
	for _, m := range s.MonthlyIn {
		d.MonthlyIn = append(d.MonthlyIn, domain.MonthAmount{
			Month:  fmt.Sprintf("%04d-%02d", m.Year, m.Month),
			Amount: p.kop("monthly_in", m.Amount),
		})
	}
	return d, p.err
}

type financials struct {
	INN              string `json:"inn"`
	Year             int    `json:"year"`
	Revenue          string `json:"revenue"`
	NetProfit        string `json:"net_profit"`
	LiabilitiesLong  string `json:"liabilities_long"`
	LiabilitiesShort string `json:"liabilities_short"`
	BalanceTotal     string `json:"balance_total"`
}

func (f *financials) domain() (*domain.Financials, error) {
	var p parser
	d := &domain.Financials{
		INN:              f.INN,
		Year:             f.Year,
		Revenue:          p.kop("revenue", f.Revenue),
		NetProfit:        p.kop("net_profit", f.NetProfit),
		LiabilitiesLong:  p.kop("liabilities_long", f.LiabilitiesLong),
		LiabilitiesShort: p.kop("liabilities_short", f.LiabilitiesShort),
		BalanceTotal:     p.kop("balance_total", f.BalanceTotal),
	}
	return d, p.err
}

// date переводит 01.10.2025 в 2025-10-01. Что не разобралось, оставляем как
// есть: проверка в domain покажет это клиенту как ошибку периода.
func date(s string) string {
	if t, err := time.Parse("02.01.2006", s); err == nil {
		return t.Format(time.DateOnly)
	}
	return s
}

// parser запоминает первую сумму, которая не разобралась
type parser struct{ err error }

// kop: «1 234 567,89», «1234567.89», «(350 475)» и «-350 475» в копейки, скобки это минус
func (p *parser) kop(field, s string) int64 {
	t := strings.Join(strings.Fields(s), "")
	neg := strings.HasPrefix(t, "-") || strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")")
	whole, frac, _ := strings.Cut(strings.Replace(strings.Trim(t, "-()"), ",", ".", 1), ".")
	v, err := strconv.ParseInt(whole+(frac + "00")[:2], 10, 64)
	if err != nil || whole == "" || len(frac) > 2 {
		if p.err == nil {
			p.err = fmt.Errorf("%w: %s: сумма %q не разбирается", domain.ErrInvalid, field, s)
		}
		return 0
	}
	if neg {
		v = -v
	}
	return v
}
