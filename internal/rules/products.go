package rules

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const bpInPct = 100

type productsFile struct {
	Version             string                 `yaml:"version"`
	KeyRatePct          float64                `yaml:"key_rate_pct"`
	AmountStepRub       int64                  `yaml:"amount_step_rub"`
	Products            map[string]productYAML `yaml:"products"`
	ProxyDocument       string                 `yaml:"proxy_document"`
	OwnTurnoverMinShare float64                `yaml:"own_turnover_min_share"`
	Goals               []goalYAML             `yaml:"goals"`
}

type productYAML struct {
	Name                 string         `yaml:"name"`
	Description          string         `yaml:"description"`
	AmountMinRub         int64          `yaml:"amount_min_rub"`
	AmountMaxRub         int64          `yaml:"amount_max_rub"`
	TermMinMonths        int            `yaml:"term_min_months"`
	TermMaxMonths        int            `yaml:"term_max_months"`
	TrancheMaxDays       int            `yaml:"tranche_max_days"`
	BaseRatePct          float64        `yaml:"base_rate_pct"`
	MaxRatePct           float64        `yaml:"max_rate_pct"`
	Repayment            string         `yaml:"repayment"`
	MinBusinessAgeMonths int            `yaml:"min_business_age_months"`
	GuarantorOverRub     int64          `yaml:"guarantor_over_rub"`
	Pledge               bool           `yaml:"pledge"`
	MaxCostShare         float64        `yaml:"max_cost_share"`
	Requirements         []string       `yaml:"requirements"`
	Documents            []documentYAML `yaml:"documents"`
}

type documentYAML struct {
	Kind          string `yaml:"kind"`
	When          string `yaml:"when"`
	Months        int    `yaml:"months"`
	AmountOverRub int64  `yaml:"amount_over_rub"`
}

type goalYAML struct {
	Code  string     `yaml:"code"`
	Title string     `yaml:"title"`
	Pick  []pickYAML `yaml:"pick"`
}

// pickYAML: условие строки пишется прямо в ней (input, lt), условие альтернативы отдельно
type pickYAML struct {
	Input         string    `yaml:"input"`
	Lt            *float64  `yaml:"lt"`
	Product       string    `yaml:"product"`
	Alternative   string    `yaml:"alternative"`
	AlternativeIf *condYAML `yaml:"alternative_if"`
}

type condYAML struct {
	Input string   `yaml:"input"`
	Lt    *float64 `yaml:"lt"`
}

// Products читает rules/products.yaml. Неизвестное поле или кривое значение это ошибка:
// опечатка в правилах должна ронять старт, а не молча менять подбор.
func Products(path string) (domain.Catalog, error) {
	return load(path, (*productsFile).catalog)
}

// load строго разбирает yaml в F и переводит в доменный вид, ошибки с путём к файлу
func load[F, R any](path string, conv func(*F) (R, error)) (R, error) {
	var (
		f   F
		res R
	)
	raw, err := os.ReadFile(path)
	if err != nil {
		return res, fmt.Errorf("rules: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err = dec.Decode(&f); err != nil {
		return res, fmt.Errorf("rules %s: %w", path, err)
	}
	if res, err = conv(&f); err != nil {
		return res, fmt.Errorf("rules %s: %w", path, err)
	}
	return res, nil
}

func (f *productsFile) catalog() (domain.Catalog, error) {
	if f.AmountStepRub <= 0 || f.KeyRatePct <= 0 {
		return domain.Catalog{}, errors.New("key_rate_pct и amount_step_rub должны быть больше 0")
	}
	c := domain.Catalog{
		Version:             f.Version,
		KeyRateBP:           bp(f.KeyRatePct),
		AmountStepKop:       f.AmountStepRub * domain.KopInRub,
		ProxyDocument:       f.ProxyDocument,
		OwnTurnoverMinShare: f.OwnTurnoverMinShare,
	}
	var err error
	if c.Products, err = products(f.Products); err != nil {
		return c, err
	}
	for _, g := range f.Goals {
		dg, err := g.goal()
		if err != nil {
			return c, fmt.Errorf("цель %s: %w", g.Code, err)
		}
		c.Goals = append(c.Goals, dg)
	}
	for _, g := range domain.Goals {
		if _, ok := c.Goal(g); !ok {
			return c, fmt.Errorf("нет цели %s", g)
		}
	}
	return c, nil
}

func products(raw map[string]productYAML) ([]domain.Product, error) {
	for code := range raw {
		if !known(code) {
			return nil, fmt.Errorf("неизвестный продукт %q", code)
		}
	}
	res := make([]domain.Product, 0, len(domain.ProductCodes))
	for _, code := range domain.ProductCodes {
		p, ok := raw[string(code)]
		if !ok {
			return nil, fmt.Errorf("нет продукта %s", code)
		}
		dp, err := p.product(code)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", code, err)
		}
		res = append(res, dp)
	}
	return res, nil
}

func (p *productYAML) product(code domain.ProductCode) (domain.Product, error) {
	switch {
	case p.AmountMinRub <= 0 || p.AmountMinRub > p.AmountMaxRub:
		return domain.Product{}, errors.New("кривой диапазон суммы")
	case p.TermMinMonths <= 0 || p.TermMinMonths > p.TermMaxMonths:
		return domain.Product{}, errors.New("кривой диапазон срока")
	case p.BaseRatePct <= 0 || p.BaseRatePct > p.MaxRatePct:
		return domain.Product{}, errors.New("кривой диапазон ставки")
	case p.Repayment != string(domain.Annuity) && p.Repayment != string(domain.AutoDebit):
		return domain.Product{}, fmt.Errorf("неизвестное погашение %q", p.Repayment)
	case p.MaxCostShare < 0 || p.MaxCostShare > 1:
		return domain.Product{}, errors.New("max_cost_share вне 0..1")
	}
	dp := domain.Product{
		Code:                 code,
		Name:                 p.Name,
		Description:          p.Description,
		AmountMinKop:         p.AmountMinRub * domain.KopInRub,
		AmountMaxKop:         p.AmountMaxRub * domain.KopInRub,
		TermMinMonths:        p.TermMinMonths,
		TermMaxMonths:        p.TermMaxMonths,
		TrancheMaxDays:       p.TrancheMaxDays,
		BaseRateBP:           bp(p.BaseRatePct),
		MaxRateBP:            bp(p.MaxRatePct),
		Repayment:            domain.Repayment(p.Repayment),
		MinBusinessAgeMonths: p.MinBusinessAgeMonths,
		GuarantorOverKop:     p.GuarantorOverRub * domain.KopInRub,
		Pledge:               p.Pledge,
		MaxCostShare:         p.MaxCostShare,
		Requirements:         p.Requirements,
	}
	for _, d := range p.Documents {
		dp.Documents = append(dp.Documents, domain.DocumentRule{
			Kind:          d.Kind,
			When:          d.When,
			Months:        d.Months,
			AmountOverKop: d.AmountOverRub * domain.KopInRub,
		})
	}
	return dp, nil
}

func (g *goalYAML) goal() (domain.GoalRule, error) {
	if !slices.Contains(domain.Goals, domain.Goal(g.Code)) {
		return domain.GoalRule{}, errors.New("неизвестная цель")
	}
	r := domain.GoalRule{Goal: domain.Goal(g.Code), Title: g.Title}
	for i, p := range g.Pick {
		when, err := condition(p.Input, p.Lt)
		if err != nil {
			return r, err
		}
		var altIf *domain.Condition
		if p.AlternativeIf != nil {
			if altIf, err = condition(p.AlternativeIf.Input, p.AlternativeIf.Lt); err != nil {
				return r, err
			}
		}
		if !known(p.Product) || (p.Alternative != "" && !known(p.Alternative)) {
			return r, fmt.Errorf("строка %d: неизвестный продукт", i+1)
		}
		r.Picks = append(r.Picks, domain.Pick{
			When:          when,
			Product:       domain.ProductCode(p.Product),
			Alternative:   domain.ProductCode(p.Alternative),
			AlternativeIf: altIf,
		})
	}
	// без строки "всё остальное" часть запросов осталась бы без рекомендации
	if len(r.Picks) == 0 || r.Picks[len(r.Picks)-1].When != nil {
		return r, errors.New("последняя строка подбора должна быть без условия")
	}
	return r, nil
}

func condition(input string, lt *float64) (*domain.Condition, error) {
	if input == "" && lt == nil {
		return nil, nil
	}
	in := domain.Input(input)
	if lt == nil || (in != domain.InputTermMonths && in != domain.InputAmountRub && in != domain.InputAmountToIncome) {
		return nil, fmt.Errorf("кривое условие %q", input)
	}
	return &domain.Condition{Input: in, Lt: *lt}, nil
}

func known(code string) bool {
	return slices.Contains(domain.ProductCodes, domain.ProductCode(code))
}

func bp(pct float64) int {
	return int(math.Round(pct * bpInPct))
}
