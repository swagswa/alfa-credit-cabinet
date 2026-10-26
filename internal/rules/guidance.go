package rules

import (
	"errors"
	"fmt"
	"math"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func Guidance(path string) (domain.GuidanceRules, error) {
	return load(path, func(r *domain.GuidanceRules) (domain.GuidanceRules, error) {
		if r.Version == "" || len(r.Improvements) == 0 {
			return *r, errors.New("нужны версия и сценарии улучшения")
		}
		seen := map[string]bool{}
		for _, step := range r.Improvements {
			v, ok := (&domain.Facts{}).Input(step.Input)
			_, numeric := v.(float64)
			if !ok || !numeric || seen[step.Input] || step.Title == "" || step.Detail == "" ||
				math.IsNaN(step.Target) ||
				math.IsInf(step.Target, 0) ||
				step.Target < 0 {
				return *r, fmt.Errorf("неверный сценарий %s", step.Input)
			}
			seen[step.Input] = true
		}
		for _, p := range domain.ProductCodes {
			m := r.RevenueMultipliers[p]
			if m <= 0 || math.IsNaN(m) || math.IsInf(m, 0) {
				return *r, fmt.Errorf("неверный множитель %s", p)
			}
		}
		return *r, nil
	})
}
