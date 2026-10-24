package usecase

import (
	"context"
	"fmt"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// На стенде тут клиент мока.
type EGRUL interface {
	Company(ctx context.Context, inn string) (domain.Company, error)
}

type CompanyUseCase interface {
	// Lookup отдаёт выписку для автоподстановки. Неверный ИНН до источника не доходит.
	Lookup(ctx context.Context, inn string) (domain.Company, error)
}

type companyUseCase struct {
	egrul EGRUL
}

func NewCompany(egrul EGRUL) CompanyUseCase {
	return &companyUseCase{egrul: egrul}
}

func (u *companyUseCase) Lookup(ctx context.Context, inn string) (domain.Company, error) {
	// схема в спеке проверяет только длину, контрольные цифры здесь
	if !domain.ValidINN(inn) {
		return domain.Company{}, fmt.Errorf("%w: ИНН %s не проходит проверку контрольных цифр", domain.ErrInvalid, inn)
	}
	return u.egrul.Company(ctx, inn)
}
