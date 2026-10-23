package usecase

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

// Sources ходит в источники проверок, на стенде integration/source.
type Sources interface {
	Fetch(ctx context.Context, src domain.Source, inn string, consentID uuid.UUID) (json.RawMessage, error)
}

type CheckUseCase interface {
	// Fetch спрашивает источник и записывает ответ. Недоступный источник
	// не записывается, а отдаётся ошибкой: его повторит temporal.
	Fetch(ctx context.Context, id uuid.UUID, src domain.Source) (domain.CheckStatus, error)
	// SaveUnavailable записывает, что источник так и не ответил.
	SaveUnavailable(ctx context.Context, id uuid.UUID, src domain.Source, reason string) error
}

type checkUseCase struct {
	apps    repo.ApplicationRepository
	checks  repo.CheckRepository
	sources Sources
}

func NewCheck(apps repo.ApplicationRepository, checks repo.CheckRepository, sources Sources) CheckUseCase {
	return &checkUseCase{apps: apps, checks: checks, sources: sources}
}

func (u *checkUseCase) Fetch(ctx context.Context, id uuid.UUID, src domain.Source) (domain.CheckStatus, error) {
	app, err := u.apps.Card(ctx, id, nil)
	if err != nil {
		return "", err
	}
	// согласие на БКИ дано в анкете, отдельного id у него нет, отдаём id заявки
	payload, err := u.sources.Fetch(ctx, src, app.Company.INN, app.ID)
	r := domain.CheckResult{Source: src, Status: domain.CheckOK, Payload: payload}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		r = domain.CheckResult{Source: src, Status: domain.CheckNotFound}
	case err != nil:
		return "", err
	}
	if err := u.checks.Add(ctx, id, r); err != nil {
		return "", err
	}
	return r.Status, nil
}

func (u *checkUseCase) SaveUnavailable(ctx context.Context, id uuid.UUID, src domain.Source, reason string) error {
	return u.checks.Add(ctx, id, domain.CheckResult{Source: src, Status: domain.CheckUnavailable, Error: reason})
}
