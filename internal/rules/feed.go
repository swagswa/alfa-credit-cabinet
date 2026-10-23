package rules

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type feedFile struct {
	Version string                   `yaml:"version"`
	Events  map[string]domain.Phrase `yaml:"events"`
	Tips    []tipYAML                `yaml:"tips"`
}

type tipYAML struct {
	Code  string        `yaml:"code"`
	Title string        `yaml:"title"`
	Text  string        `yaml:"text"`
	When  []tipCondYAML `yaml:"when"`
}

type tipCondYAML struct {
	Input string   `yaml:"input"`
	Eq    *string  `yaml:"eq"`
	Ne    *string  `yaml:"ne"`
	Gt    *float64 `yaml:"gt"`
	Lt    *float64 `yaml:"lt"`
}

// Feed читает rules/feed.yaml. Как и с продуктами, кривое правило роняет старт.
func Feed(path string) (domain.FeedRules, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.FeedRules{}, fmt.Errorf("rules: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var f feedFile
	if err = dec.Decode(&f); err != nil {
		return domain.FeedRules{}, fmt.Errorf("rules %s: %w", path, err)
	}
	r, err := f.rules()
	if err != nil {
		return domain.FeedRules{}, fmt.Errorf("rules %s: %w", path, err)
	}
	return r, nil
}

func (f *feedFile) rules() (domain.FeedRules, error) {
	r := domain.FeedRules{Version: f.Version, Events: map[domain.Status]domain.Phrase{}}
	for s, ph := range f.Events {
		st := domain.Status(s)
		if !slices.Contains(domain.Statuses, st) || ph.Title == "" || ph.Text == "" {
			return r, fmt.Errorf("events: кривая фраза для %q", s)
		}
		r.Events[st] = ph
	}
	// черновик в журнал не пишется, на остальные статусы клиент должен видеть фразу, а не код
	for _, s := range domain.Statuses {
		if _, ok := r.Events[s]; !ok && s != domain.StatusDraft {
			return r, fmt.Errorf("events: нет фразы для %s", s)
		}
	}
	for _, t := range f.Tips {
		if slices.ContainsFunc(r.Tips, func(d domain.Tip) bool { return d.Code == t.Code }) {
			return r, fmt.Errorf("tips: %s повторяется", t.Code)
		}
		dt, err := t.tip()
		if err != nil {
			return r, fmt.Errorf("tips %s: %w", t.Code, err)
		}
		r.Tips = append(r.Tips, dt)
	}
	return r, nil
}

func (t *tipYAML) tip() (domain.Tip, error) {
	dt := domain.Tip{Code: t.Code, Title: t.Title, Text: t.Text}
	if t.Code == "" || t.Title == "" || t.Text == "" {
		return dt, errors.New("нужны code, title и text")
	}
	for _, c := range t.When {
		dc, err := c.cond()
		if err != nil {
			return dt, err
		}
		dt.When = append(dt.When, dc)
	}
	return dt, nil
}

// cond: ровно один оператор, gt или lt у оборота, eq или ne у остальных
func (c *tipCondYAML) cond() (domain.TipCond, error) {
	dc := domain.TipCond{Input: domain.FeedInput(c.Input)}
	n := 0
	if c.Eq != nil {
		dc.Op, dc.Value, n = domain.OpEq, *c.Eq, n+1
	}
	if c.Ne != nil {
		dc.Op, dc.Value, n = domain.OpNe, *c.Ne, n+1
	}
	if c.Gt != nil {
		dc.Op, dc.Num, n = domain.OpGt, *c.Gt, n+1
	}
	if c.Lt != nil {
		dc.Op, dc.Num, n = domain.OpLt, *c.Lt, n+1
	}
	numeric := dc.Op == domain.OpGt || dc.Op == domain.OpLt
	var ok bool
	switch dc.Input {
	case domain.InputTurnover:
		ok = numeric
	case domain.InputOkvedClass, domain.InputProducts:
		ok = !numeric && dc.Value != ""
	case domain.InputApplication:
		ok = dc.Value == domain.NoApplication || slices.Contains(domain.Statuses, domain.Status(dc.Value))
	default:
		return dc, fmt.Errorf("неизвестный input %q", c.Input)
	}
	if n != 1 || !ok {
		return dc, fmt.Errorf("%s: нужен один подходящий оператор gt|lt|eq|ne и значение", c.Input)
	}
	return dc, nil
}
