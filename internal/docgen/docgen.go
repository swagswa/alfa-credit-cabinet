// Package docgen рисует синтетические выписки и отчётность по ИНН и эталон
// полей к каждому документу. Цифры берутся из synth, как и у моков, поэтому
// документ сходится с ответом мока ФНС, кроме ловушки.
package docgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

const (
	KindStatement  = "statement"
	KindFinancials = "financials"
)

type Doc struct {
	INN    string
	Kind   string // statement или financials
	Format string // a, b текстовые PDF разных банков, c скан
	Trap   bool   // приход в выписке завышен в 1,5 раза: суммы сходятся, с ФНС нет
	PDF    bool   // скан завёрнут в PDF
}

func (d Doc) Name() string {
	n := d.INN + "_" + d.Kind + "_" + d.Format
	if d.Trap {
		n += "_trap"
	}
	return n
}

func (d Doc) Ext() string {
	if d.Format == "c" && !d.PDF {
		return ".png"
	}
	return ".pdf"
}

// Generate отдаёт файл документа и эталон в JSON. Тот же Doc даёт те же байты.
func Generate(d Doc) (file, truth []byte, err error) {
	c, err := synth.New(d.INN)
	if err != nil {
		return nil, nil, fmt.Errorf("docgen: %w", err)
	}
	if d.Format != "a" && d.Format != "b" && d.Format != "c" {
		return nil, nil, fmt.Errorf("неизвестный формат %q", d.Format)
	}

	var (
		p page
		v any
	)
	switch d.Kind {
	case KindStatement:
		if d.Trap {
			c = inflate(c)
		}
		p, v = statementA(c), statementTruth(c)
		if d.Format == "b" {
			p = statementB(c)
		}
	case KindFinancials:
		if d.Format != "a" || d.Trap || c.IsIP() {
			return nil, nil, errors.New("отчётность только формата a и не у ИП: у ИП декларация, её не распознаём")
		}
		p, v = financialsA(c), financialsTruth(c)
	default:
		return nil, nil, fmt.Errorf("неизвестный вид документа %q", d.Kind)
	}

	if d.Format == "c" {
		file, err = scan(p, d.Name(), d.PDF)
	} else {
		file, err = renderPDF(p)
	}
	if err != nil {
		return nil, nil, err
	}
	truth, err = json.MarshalIndent(v, "", "  ")
	return file, append(truth, '\n'), err
}

// inflate это ловушка: все суммы в 1,5 раза больше, документ внутренне согласован
func inflate(c synth.Company) synth.Company {
	months := make([]synth.Month, len(c.Months))
	for i, m := range c.Months {
		months[i] = synth.Month{Start: m.Start, In: m.In * 3 / 2, Out: m.Out * 3 / 2}
	}
	c.Months, c.Opening = months, c.Opening*3/2
	return c
}

// Statement и Financials повторяют JSON-схемы, по которым модель отдаёт поля
// (internal/integration/ollama/schema): суммы в рублях с копейками.
type Statement struct {
	INN            string        `json:"inn"`
	PeriodFrom     string        `json:"period_from"`
	PeriodTo       string        `json:"period_to"`
	OpeningBalance Rub           `json:"opening_balance"`
	TurnoverIn     Rub           `json:"turnover_in"`
	TurnoverOut    Rub           `json:"turnover_out"`
	ClosingBalance Rub           `json:"closing_balance"`
	MonthlyIn      []MonthAmount `json:"monthly_in"`
}

type MonthAmount struct {
	Month  string `json:"month"`
	Amount Rub    `json:"amount"`
}

type Financials struct {
	INN              string `json:"inn"`
	Year             int    `json:"year"`
	Revenue          Rub    `json:"revenue"`
	NetProfit        Rub    `json:"net_profit"`
	LiabilitiesLong  Rub    `json:"liabilities_long"`
	LiabilitiesShort Rub    `json:"liabilities_short"`
	BalanceTotal     Rub    `json:"balance_total"`
}

func statementTruth(c synth.Company) Statement {
	s := Statement{
		INN:            c.INN,
		PeriodFrom:     c.Months[0].Start.Format("2006-01-02"),
		PeriodTo:       c.Months[len(c.Months)-1].Start.AddDate(0, 1, -1).Format("2006-01-02"),
		OpeningBalance: Rub(c.Opening),
		ClosingBalance: Rub(c.Closing()),
	}
	in, out := c.Turnover()
	s.TurnoverIn, s.TurnoverOut = Rub(in), Rub(out)
	for _, m := range c.Months {
		s.MonthlyIn = append(s.MonthlyIn, MonthAmount{m.Start.Format("2006-01"), Rub(m.In)})
	}
	return s
}

func financialsTruth(c synth.Company) Financials {
	return Financials{
		INN:              c.INN,
		Year:             c.Year,
		Revenue:          Rub(c.Income()),
		NetProfit:        Rub(c.NetProfit),
		LiabilitiesLong:  Rub(c.LiabilitiesLong),
		LiabilitiesShort: Rub(c.LiabilitiesShort),
		BalanceTotal:     Rub(c.BalanceTotal),
	}
}

// Rub хранит копейки, в JSON пишется рублями с двумя знаками: 1234.50
type Rub int64

// Text пишет рубли с разделителем тысяч sep и копейками после dec,
// пустой dec значит целые рубли. Знак отдельно, иначе -0,50 теряет минус.
func (r Rub) Text(sep, dec string) string {
	sign := ""
	if r < 0 {
		sign, r = "-", -r
	}
	s := strconv.FormatInt(int64(r/100), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + sep + s[i:]
	}
	if dec == "" {
		return sign + s
	}
	return fmt.Sprintf("%s%s%s%02d", sign, s, dec, r%100)
}

func (r Rub) MarshalJSON() ([]byte, error) { return []byte(r.Text("", ".")), nil }

func (r *Rub) UnmarshalJSON(b []byte) error {
	s := string(b)
	neg := strings.HasPrefix(s, "-")
	whole, frac, _ := strings.Cut(strings.TrimPrefix(s, "-"), ".")
	frac = (frac + "00")[:2]
	v, err := strconv.ParseInt(whole+frac, 10, 64)
	if err != nil {
		return fmt.Errorf("сумма %s: %w", s, err)
	}
	if neg {
		v = -v
	}
	*r = Rub(v)
	return nil
}
