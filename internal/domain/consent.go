package domain

import (
	"time"

	"github.com/google/uuid"
)

// Согласие на запрос в бюро действует полгода с даты получения (218-ФЗ, ст. 6).
const BKIConsentMonths = 6

// Согласие на запрос кредитной истории. Бюро смотрит
// на id и дату получения, срок считает само по тому же правилу.
type BKIConsent struct {
	ID      uuid.UUID
	GivenAt time.Time
}

func (c BKIConsent) ExpiresAt() time.Time { return c.GivenAt.AddDate(0, BKIConsentMonths, 0) }

// Valid говорит, что согласие есть и на момент now ещё не истекло.
func (c BKIConsent) Valid(now time.Time) bool {
	return c.ID != uuid.Nil && !c.GivenAt.IsZero() && now.Before(c.ExpiresAt())
}
