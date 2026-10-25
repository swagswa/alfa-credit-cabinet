package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// ClientProfile: клиент, его компании и что у них есть в банке, только для чтения.
type ClientProfile struct {
	User      User
	Companies []ProfileCompany
}

type ProfileCompany struct {
	Company
	ID   uuid.UUID
	Role RepresentativeRole
	// Bank nil и BankDown false: компания не клиент банка
	Bank     *BankAccount
	BankDown bool
	Loans    []Loan
}

type BankAccount struct {
	Number      string
	Since       time.Time
	TurnoverKop int64 // поступления за 12 месяцев
	Products    []string
}

func (b *BankAccount) Mask() string { return MaskAccount(b.Number) }

const maskTail = 4

// MaskAccount оставляет от номера счёта последние четыре цифры. Не номер (условный
// счёт выдачи не нашему клиенту) отдаёт как есть.
func MaskAccount(n string) string {
	if n == "" || strings.Trim(n, "0123456789") != "" {
		return n
	}
	return "•• " + n[max(0, len(n)-maskTail):]
}

// Loan: заявка, по которой банк выдал деньги, с условиями выдачи.
type Loan struct {
	ApplicationID uuid.UUID
	CompanyID     uuid.UUID
	Number        int64
	Status        Status
	IssuedAt      time.Time
	Terms         Terms
	ContractID    *uuid.UUID
}
