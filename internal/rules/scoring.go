package rules

import (
	"errors"
	"fmt"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// шкала 0-100: сумма максимумов факторов должна давать ровно её
const scale = 100

type scoringFile struct {
	Version    string                `yaml:"version"`
	Factors    []factorYAML          `yaml:"factors"`
	Grades     []gradeRuleYAML       `yaml:"grades"`
	TopReasons int                   `yaml:"top_reasons"`
	Reasons    map[string]reasonYAML `yaml:"reasons"`
}

type factorYAML struct {
	Code   string      `yaml:"code"`
	Name   string      `yaml:"name"`
	Input  string      `yaml:"input"`
	Bins   []binYAML   `yaml:"bins"`
	Groups []groupYAML `yaml:"groups"`
}

type binYAML struct {
	Lt     *float64 `yaml:"lt"`
	Points int      `yaml:"points"`
	Reason string   `yaml:"reason"`
}

type groupYAML struct {
	OKVED  []string `yaml:"okved"`
	Points int      `yaml:"points"`
	Reason string   `yaml:"reason"`
}

type gradeRuleYAML struct {
	Grade string `yaml:"grade"`
	Min   int    `yaml:"min"`
	PD    string `yaml:"pd"`
}

type reasonYAML struct {
	Sign string `yaml:"sign"`
	Text string `yaml:"text"`
}

// Scoring читает rules/scoring.yaml, проверки те же по духу, что у Products.
func Scoring(path string) (domain.ScoringRules, error) {
	return load(path, (*scoringFile).rules)
}

func (f *scoringFile) rules() (domain.ScoringRules, error) {
	s := domain.ScoringRules{Version: f.Version, TopReasons: f.TopReasons, Reasons: map[string]domain.ScoreReason{}}
	if f.TopReasons <= 0 {
		return s, errors.New("top_reasons должен быть больше 0")
	}
	for code, r := range f.Reasons {
		if r.Sign != "plus" && r.Sign != "minus" {
			return s, fmt.Errorf("%s: sign только plus или minus", code)
		}
		s.Reasons[code] = domain.ScoreReason{Plus: r.Sign == "plus", Text: r.Text}
	}
	total := 0
	for _, fy := range f.Factors {
		fc, err := fy.factor(s.Reasons)
		if err != nil {
			return s, fmt.Errorf("фактор %s: %w", fy.Code, err)
		}
		s.Factors = append(s.Factors, fc)
		total += fc.Max
	}
	if total != scale {
		return s, fmt.Errorf("максимумы факторов в сумме %d, а шкала 0-%d", total, scale)
	}
	for i, g := range f.Grades {
		if i > 0 && g.Min >= f.Grades[i-1].Min {
			return s, errors.New("классы должны идти по убыванию min")
		}
		s.Grades = append(s.Grades, domain.GradeRule(g))
	}
	if len(s.Grades) == 0 || s.Grades[len(s.Grades)-1].Min != 0 {
		return s, errors.New("у последнего класса должен быть min 0")
	}
	return s, nil
}

func (fy *factorYAML) factor(reasons map[string]domain.ScoreReason) (domain.Factor, error) {
	fc := domain.Factor{Code: fy.Code, Name: fy.Name, Input: fy.Input}
	v, ok := (&domain.Facts{}).Input(fy.Input)
	if !ok {
		return fc, fmt.Errorf("неизвестный input %q", fy.Input)
	}
	_, num := v.(float64)
	_, str := v.(string)
	var err error
	switch {
	case len(fy.Bins) > 0 && len(fy.Groups) == 0 && num:
		err = fy.bins(&fc, reasons)
	case len(fy.Groups) > 0 && len(fy.Bins) == 0 && str:
		err = fy.groups(&fc, reasons)
	default:
		err = errors.New("нужны либо bins для числового входа, либо groups для строкового")
	}
	return fc, err
}

func (fy *factorYAML) bins(fc *domain.Factor, reasons map[string]domain.ScoreReason) error {
	for i, b := range fy.Bins {
		if err := row(fc, reasons, i == len(fy.Bins)-1, b.Lt == nil, b.Points, b.Reason); err != nil {
			return err
		}
		if i > 0 && b.Lt != nil && *b.Lt <= *fy.Bins[i-1].Lt {
			return errors.New("lt бинов должны расти")
		}
		fc.Bins = append(fc.Bins, domain.Bin(b))
	}
	return nil
}

func (fy *factorYAML) groups(fc *domain.Factor, reasons map[string]domain.ScoreReason) error {
	for i, g := range fy.Groups {
		if err := row(fc, reasons, i == len(fy.Groups)-1, len(g.OKVED) == 0, g.Points, g.Reason); err != nil {
			return err
		}
		fc.Groups = append(fc.Groups, domain.OKVEDGroup(g))
	}
	return nil
}

// строка без границы это "всё остальное", она одна и последняя
func row(fc *domain.Factor, reasons map[string]domain.ScoreReason, last, open bool, points int, reason string) error {
	if open != last {
		return errors.New("без границы может быть только последняя строка")
	}
	if _, ok := reasons[reason]; reason != "" && !ok {
		return fmt.Errorf("нет текста для %s", reason)
	}
	fc.Max = max(fc.Max, points)
	return nil
}
