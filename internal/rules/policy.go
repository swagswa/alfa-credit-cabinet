package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type policyFile struct {
	Version           string                       `yaml:"version"`
	DSRMax            map[string]float64           `yaml:"dsr_max"`
	RatePremiumPP     map[string]float64           `yaml:"rate_premium_pp"`
	Products          map[string]productPolicyYAML `yaml:"products"`
	SwitchAgeMinShare float64                      `yaml:"switch_age_min_share"`
	LimitRoundRub     int64                        `yaml:"limit_round_rub"`
	StopFactors       []stopYAML                   `yaml:"stop_factors"`
	Reasons           map[string]string            `yaml:"reasons"`
	DocumentNames     map[string]string            `yaml:"document_names"`
}

type productPolicyYAML struct {
	AutoLimitRub int64                    `yaml:"auto_limit_rub"`
	Grades       map[string]gradeTermYAML `yaml:"grades"`
}

type gradeTermYAML struct {
	Action           domain.GradeAction `yaml:"action"`
	RevenueMult      float64            `yaml:"revenue_mult"`
	TurnoverShare    float64            `yaml:"turnover_share"`
	MaxTerm          int                `yaml:"max_term"`
	GuarantorOverRub *int64             `yaml:"guarantor_over_rub"`
}

// stopYAML: оператор это имя поля, заполнено должно быть ровно одно
type stopYAML struct {
	Code    string `yaml:"code"`
	Input   string `yaml:"input"`
	Gt      any    `yaml:"gt"`
	Lt      any    `yaml:"lt"`
	Eq      any    `yaml:"eq"`
	Ne      any    `yaml:"ne"`
	Outcome string `yaml:"outcome"`
}

// Policy читает rules/policy.yaml: суммы в копейки, надбавки в б.п.
func Policy(path string) (domain.PolicyRules, error) {
	return load(path, (*policyFile).rules)
}

func (f *policyFile) rules() (domain.PolicyRules, error) {
	p := domain.PolicyRules{
		Version:           f.Version,
		DSRMax:            f.DSRMax,
		RatePremiumBP:     map[string]int{},
		Products:          map[domain.ProductCode]domain.ProductPolicy{},
		SwitchAgeMinShare: f.SwitchAgeMinShare,
		LimitRoundKop:     f.LimitRoundRub * domain.KopInRub,
		Texts:             f.Reasons,
		DocumentNames:     f.DocumentNames,
	}
	if !share(f.SwitchAgeMinShare) || f.LimitRoundRub <= 0 {
		return p, errors.New("switch_age_min_share должен быть в (0, 1], limit_round_rub больше 0")
	}
	for g, v := range f.DSRMax {
		if !share(v) {
			return p, fmt.Errorf("dsr_max %s вне (0, 1]", g)
		}
	}
	for g, pp := range f.RatePremiumPP {
		p.RatePremiumBP[g] = bp(pp)
	}
	for code, py := range f.Products {
		if !known(code) {
			return p, fmt.Errorf("неизвестный продукт %q", code)
		}
		pp, err := py.policy()
		if err != nil {
			return p, fmt.Errorf("%s: %w", code, err)
		}
		p.Products[domain.ProductCode(code)] = pp
	}
	if len(p.Products) != len(domain.ProductCodes) {
		return p, errors.New("нужна политика для каждого продукта")
	}
	for _, s := range f.StopFactors {
		r, err := s.rule()
		if err != nil {
			return p, fmt.Errorf("стоп-фактор %s: %w", s.Code, err)
		}
		p.StopFactors = append(p.StopFactors, r)
	}
	return p, nil
}

func (py *productPolicyYAML) policy() (domain.ProductPolicy, error) {
	pp := domain.ProductPolicy{AutoLimitKop: py.AutoLimitRub * domain.KopInRub, Grades: map[string]domain.GradeTerms{}}
	if py.AutoLimitRub <= 0 {
		return pp, errors.New("auto_limit_rub должен быть больше 0")
	}
	grades := []string{"A", "B", "C", "D", "E"}
	if len(py.Grades) != len(grades) {
		return pp, errors.New("нужны ровно классы A, B, C, D, E")
	}
	for _, g := range grades {
		gy, ok := py.Grades[g]
		if !ok {
			return pp, fmt.Errorf("нет класса %s", g)
		}
		loan := gy.RevenueMult > 0 && gy.MaxTerm > 0 && gy.TurnoverShare == 0
		overdraft := share(gy.TurnoverShare) && gy.RevenueMult == 0 && gy.MaxTerm == 0
		switch {
		case gy.Action != domain.GradeAuto && gy.Action != domain.GradeManual && gy.Action != domain.GradeReject:
			return pp, fmt.Errorf("класс %s: неизвестное действие %q", g, gy.Action)
		case gy.Action != domain.GradeReject && !loan && !overdraft:
			return pp, fmt.Errorf("класс %s: нужны revenue_mult и max_term или turnover_share", g)
		case gy.GuarantorOverRub != nil && *gy.GuarantorOverRub < 0:
			return pp, fmt.Errorf("класс %s: guarantor_over_rub меньше 0", g)
		}
		gt := domain.GradeTerms{
			Action:        gy.Action,
			RevenueMult:   gy.RevenueMult,
			TurnoverShare: gy.TurnoverShare,
			MaxTerm:       gy.MaxTerm,
		}
		if gy.GuarantorOverRub != nil {
			kop := *gy.GuarantorOverRub * domain.KopInRub
			gt.GuarantorOverKop = &kop
		}
		pp.Grades[g] = gt
	}
	return pp, nil
}

func (s *stopYAML) rule() (domain.StopRule, error) {
	r := domain.StopRule{Code: s.Code, Input: s.Input}
	r.Outcome = map[string]domain.Status{
		"reject":        domain.StatusRejected,
		"need_info":     domain.StatusNeedInfo,
		"manual_review": domain.StatusManualReview,
	}[s.Outcome]
	if r.Outcome == "" {
		return r, fmt.Errorf("неизвестный исход %q", s.Outcome)
	}
	fact, ok := (&domain.Facts{}).Input(s.Input)
	if !ok {
		return r, fmt.Errorf("неизвестный input %q", s.Input)
	}
	n := 0
	for op, v := range map[domain.Op]any{domain.OpGt: s.Gt, domain.OpLt: s.Lt, domain.OpEq: s.Eq, domain.OpNe: s.Ne} {
		if v != nil {
			r.Op, r.Value = op, v
			n++
		}
	}
	if n != 1 {
		return r, errors.New("нужен ровно один оператор из gt, lt, eq, ne")
	}
	// целые yaml отдаёт как int, а числовые входы у фактов float64
	if i, ok := r.Value.(int); ok {
		r.Value = float64(i)
	}
	_, num := fact.(float64)
	switch {
	case reflect.TypeOf(r.Value) != reflect.TypeOf(fact):
		return r, fmt.Errorf("значение %v не того типа, что input", r.Value)
	case (r.Op == domain.OpGt || r.Op == domain.OpLt) && !num:
		return r, errors.New("gt и lt только для числовых входов")
	}
	return r, nil
}

func share(v float64) bool {
	return v > 0 && v <= 1
}
