package domain

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/google/uuid"
)

// MaxDocSize как CHECK в documents.size_bytes
const MaxDocSize = 10 << 20

type DocKind string

// Распознаём только эти два вида, остальные документы просто файлы.
const (
	DocStatement  DocKind = "statement"
	DocFinancials DocKind = "financials"
)

// Recognizable значит, что итоги есть только у выписки и отчётности, их и распознаём, и подтверждаем.
func (k DocKind) Recognizable() bool { return k == DocStatement || k == DocFinancials }

type DocStatus string

const (
	DocUploaded    DocStatus = "uploaded"
	DocRecognizing DocStatus = "recognizing"
	DocRecognized  DocStatus = "recognized"
	DocFailed      DocStatus = "failed"
	DocSkipped     DocStatus = "skipped"
	DocConfirmed   DocStatus = "confirmed"
)

type TextSource string

const (
	TextPDF TextSource = "pdf_text"
	TextOCR TextSource = "ocr"
)

// DocFile это загруженный документ и ИНН заявки, с которым его сверяем.
type DocFile struct {
	Kind        DocKind
	Status      DocStatus
	FileName    string
	ContentType string
	Content     []byte
	INN         string
}

// Document это карточка документа без самого файла.
type Document struct {
	ID            uuid.UUID
	ApplicationID uuid.UUID
	Kind          DocKind
	Status        DocStatus
	FileName      string
	ContentType   string
	Size          int
	SHA256        []byte
	INN           string
	TextSource    TextSource
	Extracted     *DocValues
	Confirmed     *DocValues
	EditedFields  []string
	Problems      []FieldError
	Error         string
	Model         string
	CreatedAt     time.Time
	ConfirmedAt   *time.Time
}

// Statement это итоги выписки, суммы в копейках. JSON такой же, как
// StatementValues в api/openapi.yaml: так он и лежит в documents.extracted.
type Statement struct {
	INN            string        `json:"inn"`
	PeriodFrom     string        `json:"period_from"`
	PeriodTo       string        `json:"period_to"`
	OpeningBalance int64         `json:"opening_balance_kop"`
	TurnoverIn     int64         `json:"turnover_in_kop"`
	TurnoverOut    int64         `json:"turnover_out_kop"`
	ClosingBalance int64         `json:"closing_balance_kop"`
	MonthlyIn      []MonthAmount `json:"monthly_in"`
}

type MonthAmount struct {
	Month  string `json:"month"`
	Amount int64  `json:"amount_kop"`
}

// Financials это баланс и ОФР, упрощённая форма, суммы в копейках.
type Financials struct {
	INN              string `json:"inn"`
	Year             int    `json:"year"`
	Revenue          int64  `json:"revenue_kop"`
	NetProfit        int64  `json:"net_profit_kop"`
	LiabilitiesLong  int64  `json:"liabilities_long_kop"`
	LiabilitiesShort int64  `json:"liabilities_short_kop"`
	BalanceTotal     int64  `json:"balance_total_kop"`
}

// DocValues заполнен одним блоком, под вид документа.
type DocValues struct {
	Statement  *Statement  `json:"statement,omitempty"`
	Financials *Financials `json:"financials,omitempty"`
}

// Check проверяет заполненный блок, пустые значения проверять нечего.
func (v DocValues) Check(inn string, today time.Time) []FieldError {
	switch {
	case v.Statement != nil:
		return v.Statement.Check(inn, today)
	case v.Financials != nil:
		return v.Financials.Check(inn, today)
	}
	return nil
}

// Fits значит, что заполнен ровно один блок, и он под вид документа.
func (v DocValues) Fits(k DocKind) bool {
	return (v.Statement != nil) != (v.Financials != nil) &&
		(k == DocStatement) == (v.Statement != nil) && k.Recognizable()
}

// Edited отдаёт поля, которые клиент поменял относительно распознанных.
// Помесячные поступления сравниваем целиком, это одно поле формы.
func (v DocValues) Edited(from *DocValues) []string {
	if from == nil {
		return nil
	}
	a, b := v.fields(), from.fields()
	var out []string
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if !reflect.DeepEqual(a[k], b[k]) {
			out = append(out, k)
		}
	}
	return out
}

// fields раскладывает заполненный блок по именам полей из JSON, они же имена в форме
func (v DocValues) fields() map[string]any {
	var m map[string]map[string]any
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &m)
	for _, f := range m {
		return f
	}
	return nil
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Recognition это всё, что сохраняем для аудита: откуда текст, сырой ответ
// модели, поля после разбора, что не прошло проверку и на какой модели.
type Recognition struct {
	Source   TextSource
	Raw      []byte
	Values   DocValues
	Problems []FieldError
	Model    string
	Digest   string
	Version  string // версия промпта, схемы и предобработки
	Took     time.Duration
}

const (
	// допуск на округление: модель и OCR теряют копейки, рубль не страшно
	sumTolerance = 100
	maxMonths    = 13
	// отчётность за прошлый или позапрошлый год
	maxReportAge = 2
)

// Check сверяет выписку с заявкой и саму с собой.
func (s *Statement) Check(inn string, today time.Time) []FieldError {
	var errs FieldErrors
	add := errs.add
	if s.INN != inn {
		add("inn", "ИНН в документе %s, в заявке %s", s.INN, inn)
	}
	if s.TurnoverIn <= 0 {
		add("turnover_in_kop", "нет поступлений за период")
	}
	if s.TurnoverOut < 0 {
		add("turnover_out_kop", "списания меньше нуля")
	}
	if diff(s.OpeningBalance+s.TurnoverIn-s.TurnoverOut, s.ClosingBalance) > sumTolerance {
		add("closing_balance_kop", "остаток на начало плюс приход минус расход не равен остатку на конец")
	}

	from, errFrom := time.Parse(time.DateOnly, s.PeriodFrom)
	to, errTo := time.Parse(time.DateOnly, s.PeriodTo)
	if errFrom != nil || errTo != nil {
		add("period_from", "период %q - %q не похож на даты", s.PeriodFrom, s.PeriodTo)
		return errs
	}
	n := months(from, to)
	switch {
	case !from.Before(to):
		add("period_from", "начало периода не раньше конца")
	case n > maxMonths:
		add("period_from", "период %d мес., выписку берём не больше чем за %d", n, maxMonths)
	case to.After(today):
		add("period_to", "период заканчивается в будущем")
	}

	var sum int64
	for i, m := range s.MonthlyIn {
		sum += m.Amount
		if want := from.AddDate(0, i, 0).Format("2006-01"); m.Month != want {
			add("monthly_in", "на месте %d-го месяца %q, ждали %s", i+1, m.Month, want)
			return errs
		}
	}
	if len(s.MonthlyIn) != n {
		add("monthly_in", "помесячных строк %d, а в периоде %d мес.", len(s.MonthlyIn), n)
	}
	if diff(sum, s.TurnoverIn) > sumTolerance*int64(len(s.MonthlyIn)) {
		add("monthly_in", "сумма по месяцам не равна поступлениям за период")
	}
	return errs
}

// Check для отчётности: год прошлый или позапрошлый, строки формы не противоречат друг другу.
func (f *Financials) Check(inn string, today time.Time) []FieldError {
	var errs FieldErrors
	add := errs.add
	if f.INN != inn {
		add("inn", "ИНН в документе %s, в заявке %s", f.INN, inn)
	}
	if age := today.Year() - f.Year; age < 1 || age > maxReportAge {
		add("year", "отчётность за %d год, нужна за прошлый или позапрошлый", f.Year)
	}
	if f.Revenue < 0 {
		add("revenue_kop", "выручка меньше нуля")
	}
	if diff(f.NetProfit, 0) > f.Revenue {
		add("net_profit_kop", "прибыль или убыток больше выручки")
	}
	if f.LiabilitiesLong < 0 {
		add("liabilities_long_kop", "долгосрочные обязательства меньше нуля")
	}
	if f.LiabilitiesShort < 0 {
		add("liabilities_short_kop", "краткосрочные обязательства меньше нуля")
	}
	if f.BalanceTotal < f.LiabilitiesLong+f.LiabilitiesShort {
		add("balance_total_kop", "валюта баланса меньше суммы обязательств")
	}
	return errs
}

// months считает календарные месяцы периода включительно: 01.10-30.09 это 12
func months(from, to time.Time) int {
	return (to.Year()-from.Year())*12 + int(to.Month()-from.Month()) + 1
}

func diff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}
