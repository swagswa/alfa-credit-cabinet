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

// Lock для перехода заполняет только первые четыре поля.
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
	// Documents только в карточке: id, вид, статус, имя файла и подтверждённые итоги
	Documents []Document
	// Checks только для сотрудников банка, последний ответ по каждому источнику
	Checks []CheckResult
	// Decision последнее решение, только в карточке
	Decision *Decision
	// InfoRequest открытый запрос банка, только в карточке и только в need_info
	InfoRequest *InfoRequest
}

// Adjustable: встречное посчитала политика, клиент может подобрать сумму и срок сам.
func (a *Application) Adjustable() bool {
	return a.Status == StatusCounterOffer && a.Decision != nil && a.Decision.Stage == StagePolicy
}

// InfoRequest что банк ждёт от клиента. Reasons как показали клиенту.
type InfoRequest struct {
	Documents []DocKind
	Reasons   []Reason
	Comment   string
}

// ActorName пустой у системы.
type AuditEntry struct {
	ID        int64
	Kind      string
	From, To  Status
	ActorRole Role
	ActorName string
	Codes     []string
	Comment   string
	At        time.Time
}

// Анкета, клиент правит её, пока заявка в draft. Нули значат «не заполнено»,
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
	// Sort как в openapi: created_at, -amount_kop, due_at и т.д., пустой значит новые сверху
	Sort   string
	Limit  int
	Offset int
}

// Actor двигает заявку. У RoleSystem ID нулевой.
type Actor struct {
	ID   uuid.UUID
	Role Role
}

type Move struct {
	To    Status
	Actor Actor
	// Version карточки, которую видел человек. У шагов workflow 0, их не сверяем.
	Version int
	Codes   []string
	Comment string
	// DueAt срок нового шага, его считает workflow вместе с таймером. nil снимает срок.
	DueAt *time.Time
	// Decision пишется вместе с переходом, условия из него становятся текущими
	Decision *Decision
	// Documents просит сотрудник вместе с переходом в need_info
	Documents []DocKind
	// RevenueKop уточнённая выручка из ответа клиента, пишется вместе с переходом из need_info
	RevenueKop *int64
}

// InfoRequest из перехода в need_info. По стоп-фактору документы и тексты причин
// берём из решения, у остальных причины только кодами: клиент их не увидит, банк увидит.
func (m *Move) InfoRequest() InfoRequest {
	r := InfoRequest{Documents: m.Documents, Comment: m.Comment}
	if d := m.Decision; d != nil {
		r.Documents, r.Reasons = d.Documents, d.Reasons
		return r
	}
	for _, c := range m.Codes {
		r.Reasons = append(r.Reasons, Reason{Code: c})
	}
	return r
}

// Уведомление для SSE. Теги под payload функции notify_application
// из миграций, Postgres шлёт его после коммита.
type Event struct {
	ApplicationID uuid.UUID `json:"application_id"`
	CompanyID     uuid.UUID `json:"company_id"`
	Status        Status    `json:"status"`
	Version       int       `json:"version"`
	// заявку подавали хотя бы раз, только такие видит банк
	Submitted bool `json:"submitted"`
	// у события document, его шлёт триггер на documents
	DocumentID     uuid.UUID `json:"document_id"`
	DocumentStatus DocStatus `json:"document_status"`
	// у события check: ответ источника или неудачная попытка, тогда CheckStatus пустой
	Source      Source      `json:"source"`
	CheckStatus CheckStatus `json:"check_status"`
	Attempt     int         `json:"attempt"`
	Attempts    int         `json:"attempts"`
	CheckError  string      `json:"error"`
	// у события notification: кому уведомление, остальным его не шлём
	NotificationUser uuid.UUID `json:"notification_user"`
}
