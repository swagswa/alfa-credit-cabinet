package usecase

import "github.com/swagswa/alfa-credit-cabinet/internal/domain"

// FIXME: заглушка до B10, всё уходит андеррайтеру
func (u *decisionUseCase) Policy(
	_ *domain.Application,
	_ *domain.Facts,
	_ *domain.Decision,
	s *domain.Score,
) domain.Decision {
	return domain.Decision{
		Stage:         domain.StagePolicy,
		Outcome:       domain.StatusManualReview,
		Score:         s,
		PolicyVersion: u.policy.Version,
	}
}
