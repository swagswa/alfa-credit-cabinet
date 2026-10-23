package usecase

import "github.com/swagswa/alfa-credit-cabinet/internal/domain"

// FIXME: заглушка до B09
func (u *decisionUseCase) Score(_ *domain.Facts) domain.Score {
	return domain.Score{Version: u.scoring.Version}
}
