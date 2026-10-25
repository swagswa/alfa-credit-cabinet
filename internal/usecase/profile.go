package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

type ProfileUseCase interface {
	// Profile собирает профиль клиента. Данные банка спрашиваются вживую;
	// источник недоступен, профиль всё равно отдаётся, с пометкой у компании.
	Profile(ctx context.Context, userID uuid.UUID) (domain.ClientProfile, error)
}

type profileUseCase struct {
	users   repo.UserRepository
	sources Sources
}

func NewProfile(users repo.UserRepository, sources Sources) ProfileUseCase {
	return &profileUseCase{users: users, sources: sources}
}

func (u *profileUseCase) Profile(ctx context.Context, userID uuid.UUID) (domain.ClientProfile, error) {
	user, err := u.users.ByID(ctx, userID)
	if err != nil {
		return domain.ClientProfile{}, err
	}
	companies, err := u.users.Companies(ctx, userID)
	if err != nil {
		return domain.ClientProfile{}, err
	}
	loans, err := u.users.Loans(ctx, userID)
	if err != nil {
		return domain.ClientProfile{}, err
	}

	for i := range companies {
		c := &companies[i]
		for j := range loans {
			if loans[j].CompanyID == c.ID {
				c.Loans = append(c.Loans, loans[j])
			}
		}
		c.Bank, err = u.bank(ctx, c.INN)
		c.BankDown = err != nil && !errors.Is(err, domain.ErrNotFound)
	}
	return domain.ClientProfile{User: user, Companies: companies}, nil
}

// bank отдаёт nil, если компания не клиент банка
func (u *profileUseCase) bank(ctx context.Context, inn string) (*domain.BankAccount, error) {
	raw, err := u.sources.Fetch(ctx, domain.SourceInternal, inn, domain.BKIConsent{})
	if err != nil {
		return nil, err
	}
	var r bankReply
	if err = json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("банк %s: %w", inn, err)
	}
	if !r.IsClient {
		return nil, nil
	}
	since, err := time.Parse(time.DateOnly, r.Opened)
	if err != nil {
		return nil, fmt.Errorf("банк account_opened_at: %w", err)
	}
	b := &domain.BankAccount{Number: r.Account, Since: since, Products: r.Products}
	for _, m := range r.Turnover {
		b.TurnoverKop += m.Credit * domain.KopInRub
	}
	return b, nil
}
