package domain

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft             Status = "draft"
	StatusSubmitted         Status = "submitted"
	StatusChecking          Status = "checking"
	StatusNeedInfo          Status = "need_info"
	StatusScoring           Status = "scoring"
	StatusApproved          Status = "approved"
	StatusCounterOffer      Status = "counter_offer"
	StatusManualReview      Status = "manual_review"
	StatusRejected          Status = "rejected"
	StatusAccepted          Status = "accepted"
	StatusSigning           Status = "signing"
	StatusIssued            Status = "issued"
	StatusClosed            Status = "closed"
	StatusCancelledByClient Status = "cancelled_by_client"
	StatusExpired           Status = "expired"
)

// Statuses в порядке enum из openapi.
var Statuses = []Status{
	StatusDraft, StatusSubmitted, StatusChecking, StatusNeedInfo, StatusScoring,
	StatusApproved, StatusCounterOffer, StatusManualReview, StatusRejected, StatusAccepted,
	StatusSigning, StatusIssued, StatusClosed, StatusCancelledByClient, StatusExpired,
}

// Final значит, что из статуса переходов нет, новая попытка это новая заявка.
func (s Status) Final() bool {
	for _, t := range Transitions {
		if t.From == s {
			return false
		}
	}
	return true
}

// Application пока только то, что нужно переходу, остальные поля добавятся с заявками.
type Application struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	Status    Status
	Version   int
}

// Actor двигает заявку. У RoleSystem ID нулевой.
type Actor struct {
	ID   uuid.UUID
	Role Role
}

// Move это запрос на переход заявки в To.
type Move struct {
	To    Status
	Actor Actor
	// Version карточки, которую видел человек. У шагов workflow 0, их не сверяем.
	Version int
	Codes   []string
	Comment string
	// DueAt срок нового шага, его считает workflow вместе с таймером. nil снимает срок.
	DueAt *time.Time
}
