package usecase

import "github.com/swagswa/alfa-credit-cabinet/internal/domain"

// FIXME: заглушка до B08
func (u *decisionUseCase) Check(_ *domain.Facts) domain.Decision {
	return domain.Decision{Stage: domain.StageChecks}
}
