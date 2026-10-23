package domain

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type FeedKind string

const (
	FeedEvent FeedKind = "event"
	FeedTip   FeedKind = "tip"
)

// FeedItem это строка ленты клиента. ApplicationID нулевой у подсказки.
type FeedItem struct {
	ID            string
	Kind          FeedKind
	Title, Text   string
	ApplicationID uuid.UUID
	At            time.Time
}

// StatusEvent это смена статуса заявки из журнала, как её видит лента.
type StatusEvent struct {
	ID            int64
	ApplicationID uuid.UUID
	Number        int64
	Status        Status
	At            time.Time
}

// Profile это то, по чему подбираются подсказки. Bank false значит, что данных
// банка нет (заявок не было или банк не ответил): тогда условия по продуктам
// и оборотам не выполняются.
type Profile struct {
	CompanyID        uuid.UUID
	Okved            string
	LastStatus       string // NoApplication, если заявок нет
	Bank             bool
	Products         []string
	TurnoverMonthRub int64
}

const NoApplication = "none"

// FeedInput это поле профиля в условии подсказки. Набор фиксированный.
type FeedInput string

const (
	InputOkvedClass  FeedInput = "okved_class"
	InputProducts    FeedInput = "products"
	InputTurnover    FeedInput = "turnover_month_rub"
	InputApplication FeedInput = "application"
)

type Op string

const (
	OpGt Op = "gt"
	OpLt Op = "lt"
	OpEq Op = "eq"
	OpNe Op = "ne"
)

// TipCond это условие подсказки. У turnover_month_rub сравнивается Num (gt, lt), у остальных Value (eq, ne).
// products eq значит «продукт есть», ne «продукта нет».
type TipCond struct {
	Input FeedInput
	Op    Op
	Value string
	Num   float64
}

type Tip struct {
	Code, Title, Text string
	When              []TipCond
}

type Phrase struct {
	Title, Text string
}

// FeedRules из rules/feed.yaml: фразы клиенту на смену статуса и подсказки.
type FeedRules struct {
	Version string
	Events  map[Status]Phrase
	Tips    []Tip
}

// Pick отдаёт подсказки, у которых выполнены все условия, в порядке файла.
func (r *FeedRules) Pick(p *Profile) []Tip {
	var out []Tip
	for _, t := range r.Tips {
		if !slices.ContainsFunc(t.When, func(c TipCond) bool { return !c.holds(p) }) {
			out = append(out, t)
		}
	}
	return out
}

// Event переводит смену статуса в строку ленты. Статус без фразы в ленту не идёт.
func (r *FeedRules) Event(ev *StatusEvent) (FeedItem, bool) {
	ph, ok := r.Events[ev.Status]
	if !ok {
		return FeedItem{}, false
	}
	n := strconv.FormatInt(ev.Number, 10)
	return FeedItem{
		ID:            "ev-" + strconv.FormatInt(ev.ID, 10),
		Kind:          FeedEvent,
		Title:         strings.ReplaceAll(ph.Title, "{number}", n),
		Text:          strings.ReplaceAll(ph.Text, "{number}", n),
		ApplicationID: ev.ApplicationID,
		At:            ev.At,
	}, true
}

func (c TipCond) holds(p *Profile) bool {
	switch c.Input {
	case InputOkvedClass:
		class, _, _ := strings.Cut(p.Okved, ".")
		return c.is(class == c.Value)
	case InputApplication:
		return c.is(p.LastStatus == c.Value)
	case InputProducts:
		return p.Bank && c.is(slices.Contains(p.Products, c.Value))
	case InputTurnover:
		v := float64(p.TurnoverMonthRub)
		return p.Bank && (c.Op == OpGt && v > c.Num || c.Op == OpLt && v < c.Num)
	}
	return false
}

func (c TipCond) is(match bool) bool {
	return match == (c.Op == OpEq)
}
