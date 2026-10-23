package usecase

import (
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// FIXME: заглушка до B08
func (u *decisionUseCase) Facts(_ *domain.Application, _ []domain.CheckResult, _ time.Time) (domain.Facts, error) {
	return domain.Facts{}, nil
}
