package usecase

import (
	"slices"
	"strconv"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// Score считает балл по rules/scoring.yaml. Что вход фактора есть в Facts и нужного
// типа, проверил загрузчик правил.
func (u *decisionUseCase) Score(f *domain.Facts) domain.Score {
	s := domain.Score{Version: u.scoring.Version}
	for _, fc := range u.scoring.Factors {
		fs := domain.FactorScore{Code: fc.Code, Name: fc.Name, Max: fc.Max}
		v, _ := f.Input(fc.Input)
		switch v := v.(type) {
		case float64:
			fs.Value = v
			fs.Points, fs.Reason = bin(fc.Bins, v)
		case string:
			// группу ОКВЭД в карточке показываем числом, "56" это 56
			fs.Value, _ = strconv.ParseFloat(v, 64)
			fs.Points, fs.Reason = group(fc.Groups, v)
		}
		s.Points += fs.Points
		s.Factors = append(s.Factors, fs)
	}
	for _, g := range u.scoring.Grades {
		if s.Points >= g.Min {
			s.Grade, s.PD = g.Grade, g.PD
			break
		}
	}
	return s
}

// бины идут сверху вниз, последний без границы ловит всё, в том числе +Inf у нагрузки без выручки
func bin(bins []domain.Bin, v float64) (points int, reason string) {
	for _, b := range bins {
		if b.Lt == nil || v < *b.Lt {
			return b.Points, b.Reason
		}
	}
	return 0, ""
}

func group(groups []domain.OKVEDGroup, okved string) (points int, reason string) {
	for _, g := range groups {
		if len(g.OKVED) == 0 || slices.Contains(g.OKVED, okved) {
			return g.Points, g.Reason
		}
	}
	return 0, ""
}

// scoreReasons объясняет балл клиенту: до TopReasons плюсов, где набрано больше всего,
// и столько же минусов, где больше всего потеряно. Факторы без кода в бине не попадают.
func (u *decisionUseCase) scoreReasons(s *domain.Score) []domain.Reason {
	var plus, minus []domain.FactorScore
	for _, fs := range s.Factors {
		r, ok := u.scoring.Reasons[fs.Reason]
		switch {
		case !ok:
		case r.Plus:
			plus = append(plus, fs)
		default:
			minus = append(minus, fs)
		}
	}
	// стабильная сортировка: при равных баллах порядок факторов из правил
	slices.SortStableFunc(plus, func(a, b domain.FactorScore) int { return b.Points - a.Points })
	slices.SortStableFunc(minus, func(a, b domain.FactorScore) int { return (b.Max - b.Points) - (a.Max - a.Points) })

	n := u.scoring.TopReasons
	out := []domain.Reason{}
	for _, fs := range slices.Concat(plus[:min(n, len(plus))], minus[:min(n, len(minus))]) {
		out = append(out, domain.Reason{Code: fs.Reason, Text: u.scoring.Reasons[fs.Reason].Text})
	}
	return out
}
