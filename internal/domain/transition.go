package domain

import "slices"

type Edge struct {
	From, To Status
	Roles    []Role
}

// Transitions говорит, кто может перевести заявку из From в To. Порядок как в docs/state-diagram.md,
// golden-тест сверяет построчно. admin здесь нет: он ведёт правила, а не заявки.
var Transitions = []Edge{
	{StatusDraft, StatusSubmitted, []Role{RoleClient}},
	{StatusDraft, StatusCancelledByClient, []Role{RoleClient}},
	{StatusSubmitted, StatusChecking, []Role{RoleSystem}},
	{StatusChecking, StatusNeedInfo, []Role{RoleSystem, RoleManager, RoleUnderwriter}},
	{StatusChecking, StatusRejected, []Role{RoleSystem}},
	{StatusChecking, StatusManualReview, []Role{RoleSystem}},
	{StatusChecking, StatusScoring, []Role{RoleSystem}},
	{StatusNeedInfo, StatusChecking, []Role{RoleClient}},
	{StatusNeedInfo, StatusExpired, []Role{RoleSystem}},
	{StatusNeedInfo, StatusCancelledByClient, []Role{RoleClient}},
	{StatusScoring, StatusApproved, []Role{RoleSystem}},
	{StatusScoring, StatusCounterOffer, []Role{RoleSystem}},
	{StatusScoring, StatusManualReview, []Role{RoleSystem}},
	{StatusScoring, StatusRejected, []Role{RoleSystem}},
	{StatusManualReview, StatusApproved, []Role{RoleUnderwriter}},
	{StatusManualReview, StatusCounterOffer, []Role{RoleUnderwriter}},
	{StatusManualReview, StatusRejected, []Role{RoleUnderwriter}},
	{StatusManualReview, StatusNeedInfo, []Role{RoleUnderwriter, RoleManager}},
	{StatusManualReview, StatusChecking, []Role{RoleManager}},
	{StatusApproved, StatusAccepted, []Role{RoleClient}},
	{StatusApproved, StatusCancelledByClient, []Role{RoleClient}},
	{StatusApproved, StatusExpired, []Role{RoleSystem}},
	{StatusCounterOffer, StatusAccepted, []Role{RoleClient}},
	{StatusCounterOffer, StatusCancelledByClient, []Role{RoleClient}},
	{StatusCounterOffer, StatusExpired, []Role{RoleSystem}},
	{StatusAccepted, StatusSigning, []Role{RoleSystem}},
	{StatusSigning, StatusIssued, []Role{RoleClient}},
	{StatusSigning, StatusCancelledByClient, []Role{RoleClient}},
	{StatusSigning, StatusExpired, []Role{RoleSystem}},
	{StatusIssued, StatusClosed, []Role{RoleSystem}},
}

func CanTransition(from, to Status, role Role) bool {
	for _, e := range Transitions {
		if e.From == from && e.To == to {
			return slices.Contains(e.Roles, role)
		}
	}
	return false
}

// AllowedNext отдаёт, куда роль может перевести заявку из status, по нему фронт рисует кнопки.
func AllowedNext(status Status, role Role) []Status {
	var next []Status
	for _, e := range Transitions {
		if e.From == status && slices.Contains(e.Roles, role) {
			next = append(next, e.To)
		}
	}
	return next
}
