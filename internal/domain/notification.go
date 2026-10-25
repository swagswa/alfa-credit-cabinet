package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Notification: уведомление клиенту в кабинете. Kind это статус заявки или signing_code.
type Notification struct {
	ID            int64
	ApplicationID uuid.UUID
	Kind          string
	Title, Text   string
	CreatedAt     time.Time
	ReadAt        *time.Time
}

const NoticeSigningCode = "signing_code"

// NoticeRules из rules/notifications.yaml: фразы на статусы и на код подписи.
type NoticeRules struct {
	Version     string
	Statuses    map[Status]Phrase
	SigningCode Phrase
}

// Fill подставляет значения в фразу, vars парами: "{number}", "12".
func (p Phrase) Fill(vars ...string) Phrase {
	r := strings.NewReplacer(vars...)
	return Phrase{Title: r.Replace(p.Title), Text: r.Replace(p.Text)}
}

// NoticeVars: что подставляется в фразу уведомления.
type NoticeVars struct {
	Number    int64
	AmountKop int64
	Account   string
}
