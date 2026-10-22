package domain

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
