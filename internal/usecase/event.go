package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
)

type EventUseCase interface {
	// Subscribe отдаёт события, которые пользователь вправе видеть: клиент
	// только по своим компаниям, сотрудник банка все. appID, если не нулевой,
	// оставляет одну заявку.
	Subscribe(ctx context.Context, s domain.Session, appID uuid.UUID) (<-chan domain.Event, func(), error)
}

type eventUseCase struct {
	events repo.EventRepository
	users  repo.UserRepository
}

func NewEvent(events repo.EventRepository, users repo.UserRepository) EventUseCase {
	return &eventUseCase{events: events, users: users}
}

func (u *eventUseCase) Subscribe(
	ctx context.Context,
	s domain.Session,
	appID uuid.UUID,
) (events <-chan domain.Event, cancel func(), err error) {
	// компании клиента читаем один раз на соединение, а не на каждое событие:
	// только что добавленная компания попадёт в поток после переподключения
	var own map[uuid.UUID]bool
	if s.Role == domain.RoleClient {
		var ms []domain.Membership
		if ms, err = u.users.Memberships(ctx, s.UserID); err != nil {
			return nil, nil, err
		}
		own = make(map[uuid.UUID]bool, len(ms))
		for _, m := range ms {
			own[m.CompanyID] = true
		}
	}
	events, cancel = u.events.Subscribe(func(ev domain.Event) bool {
		return (appID == uuid.Nil || ev.ApplicationID == appID) && (own == nil || own[ev.CompanyID])
	})
	return events, cancel, nil
}
