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
	Fetch(ctx context.Context, src domain.Source, inn string, consent domain.BKIConsent) (json.RawMessage, error)
}

const noConsent = "бюро отказало: нет действующего согласия на запрос кредитной истории"

type CheckUseCase interface {
	// Fetch спрашивает источник и записывает ответ. Недоступный источник
	// не записывается, а отдаётся ошибкой: его повторит temporal. Отказ бюро
	// без согласия повторять бесполезно, он пишется как unavailable с причиной.
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
	// согласие нужно только бюро. Нет записи: идём с пустым, откажет само бюро
	var consent domain.BKIConsent
	if src == domain.SourceBKI {
		consent, err = u.apps.BKIConsent(ctx, id)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
	}
	payload, err := u.sources.Fetch(ctx, src, app.Company.INN, consent)
	r := domain.CheckResult{Source: src, Status: domain.CheckOK, Payload: payload}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		r = domain.CheckResult{Source: src, Status: domain.CheckNotFound}
	case errors.Is(err, domain.ErrForbidden):
		r = domain.CheckResult{Source: src, Status: domain.CheckUnavailable, Error: noConsent}
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
