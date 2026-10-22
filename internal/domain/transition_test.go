package domain_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to domain.Status
		role     domain.Role
		want     bool
	}{
		{domain.StatusDraft, domain.StatusSubmitted, domain.RoleClient, true},
		{domain.StatusSubmitted, domain.StatusChecking, domain.RoleSystem, true},
		{domain.StatusChecking, domain.StatusNeedInfo, domain.RoleManager, true},
		{domain.StatusChecking, domain.StatusNeedInfo, domain.RoleUnderwriter, true},
		{domain.StatusManualReview, domain.StatusCounterOffer, domain.RoleUnderwriter, true},
		{domain.StatusManualReview, domain.StatusChecking, domain.RoleManager, true},
		{domain.StatusSigning, domain.StatusIssued, domain.RoleClient, true},
		{domain.StatusIssued, domain.StatusClosed, domain.RoleSystem, true},

		// чужая роль на существующем ребре
		{domain.StatusDraft, domain.StatusSubmitted, domain.RoleManager, false},
		{domain.StatusManualReview, domain.StatusApproved, domain.RoleManager, false},
		{domain.StatusManualReview, domain.StatusChecking, domain.RoleUnderwriter, false},
		// system не делает шаги людей, люди не делают шаги system
		{domain.StatusDraft, domain.StatusSubmitted, domain.RoleSystem, false},
		{domain.StatusScoring, domain.StatusApproved, domain.RoleUnderwriter, false},
		{domain.StatusChecking, domain.StatusScoring, domain.RoleClient, false},
		// админ заявки не ведёт
		{domain.StatusManualReview, domain.StatusApproved, domain.RoleAdmin, false},
		{domain.StatusChecking, domain.StatusNeedInfo, domain.RoleAdmin, false},
		// рёбер нет
		{domain.StatusDraft, domain.StatusApproved, domain.RoleClient, false},
		{domain.StatusChecking, domain.StatusCancelledByClient, domain.RoleClient, false},
		{domain.StatusAccepted, domain.StatusCancelledByClient, domain.RoleClient, false},
		{domain.StatusRejected, domain.StatusChecking, domain.RoleSystem, false},
		{domain.StatusExpired, domain.StatusDraft, domain.RoleClient, false},
		{domain.StatusDraft, domain.StatusDraft, domain.RoleClient, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s->%s/%s", tt.from, tt.to, tt.role), func(t *testing.T) {
			require.Equal(t, tt.want, domain.CanTransition(tt.from, tt.to, tt.role))
		})
	}
}

func TestAllowedNext(t *testing.T) {
	tests := []struct {
		status domain.Status
		role   domain.Role
		want   []domain.Status
	}{
		{
			domain.StatusDraft,
			domain.RoleClient,
			[]domain.Status{domain.StatusSubmitted, domain.StatusCancelledByClient},
		},
		{domain.StatusChecking, domain.RoleClient, nil},
		{domain.StatusChecking, domain.RoleManager, []domain.Status{domain.StatusNeedInfo}},
		{
			domain.StatusNeedInfo,
			domain.RoleClient,
			[]domain.Status{domain.StatusChecking, domain.StatusCancelledByClient},
		},
		{domain.StatusManualReview, domain.RoleUnderwriter, []domain.Status{
			domain.StatusApproved, domain.StatusCounterOffer, domain.StatusRejected, domain.StatusNeedInfo,
		}},
		{domain.StatusManualReview, domain.RoleManager, []domain.Status{domain.StatusNeedInfo, domain.StatusChecking}},
		{domain.StatusManualReview, domain.RoleAdmin, nil},
		{
			domain.StatusCounterOffer,
			domain.RoleClient,
			[]domain.Status{domain.StatusAccepted, domain.StatusCancelledByClient},
		},
		{domain.StatusSigning, domain.RoleClient, []domain.Status{domain.StatusIssued, domain.StatusCancelledByClient}},
		{domain.StatusClosed, domain.RoleClient, nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.status, tt.role), func(t *testing.T) {
			require.Equal(t, tt.want, domain.AllowedNext(tt.status, tt.role))
		})
	}
}

func TestFinal(t *testing.T) {
	var final []domain.Status
	for _, s := range domain.Statuses {
		if s.Final() {
			final = append(final, s)
		}
	}
	require.Equal(t, []domain.Status{
		domain.StatusRejected, domain.StatusClosed, domain.StatusCancelledByClient, domain.StatusExpired,
	}, final)
}

// Диаграмма в docs пишется руками, тест держит её в соответствии с таблицей.
func TestStateDiagram(t *testing.T) {
	raw, err := os.ReadFile("../../docs/state-diagram.md")
	require.NoError(t, err)
	_, block, ok := strings.Cut(string(raw), "```mermaid\n")
	require.True(t, ok, "нет блока mermaid")
	block, _, ok = strings.Cut(block, "```")
	require.True(t, ok)

	var b strings.Builder
	b.WriteString("stateDiagram-v2\n    [*] --> draft\n")
	for _, tr := range domain.Transitions {
		fmt.Fprintf(&b, "    %s --> %s\n", tr.From, tr.To)
	}
	for _, s := range domain.Statuses {
		if s.Final() {
			fmt.Fprintf(&b, "    %s --> [*]\n", s)
		}
	}
	require.Equal(t, block, b.String())
}
