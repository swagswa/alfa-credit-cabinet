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

// Application это заявка. Lock для перехода заполняет только первые четыре поля.
type Application struct {
	ID        uuid.UUID
	CompanyID uuid.UUID
	Status    Status
	Version   int

	Number int64
	// Company из ЕГРЮЛ, в карточке только реквизиты для шапки
	Company Company
	Draft
	Score       *int
	Grade       string
	DueAt       *time.Time
	SubmittedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Draft это анкета, клиент правит её, пока заявка в draft. Нули значат «не заполнено»,
// полноту проверяет подача. Role пишется в company_members: это роль человека в компании.
type Draft struct {
	Role              RepresentativeRole
	Product           ProductCode
	Purpose           Goal
	AmountKop         int64
	TermMonths        int
	MonthlyRevenueKop int64
	EquipmentCostKop  int64
	BKIConsent        bool
	PDConsent         bool
	Comment           string
}

// ApplicationFilter для списка. Owner задан у клиента, у сотрудника банка nil.
type ApplicationFilter struct {
	Owner    *uuid.UUID
	Statuses []Status
	Grades   []string
	Product  ProductCode
	Limit    int
	Offset   int
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

// Event это уведомление об изменении заявки для SSE. Теги под payload
// pg_notify из queries/applications.sql, Postgres шлёт его после коммита.
type Event struct {
	ApplicationID uuid.UUID `json:"application_id"`
	CompanyID     uuid.UUID `json:"company_id"`
	Status        Status    `json:"status"`
	Version       int       `json:"version"`
}
