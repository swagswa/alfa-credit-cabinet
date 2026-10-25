package domain

import "github.com/google/uuid"

type Role string

const (
	RoleClient      Role = "client"
	RoleManager     Role = "manager"
	RoleUnderwriter Role = "underwriter"
	RoleAdmin       Role = "admin"
	// так в таблице переходов и аудите записан ApplicationWorkflow, учёток с ней нет
	RoleSystem Role = "system"
)

type User struct {
	ID       uuid.UUID
	Login    string
	FullName string
	Role     Role
	Phone    string
	Email    string
}

// AlfaProfile отдаёт Альфа ID после согласия клиента. Логин тот же, что в кабинете.
type AlfaProfile struct {
	Login, Phone, Email string
}

// RepresentativeRole говорит, на каком основании клиент действует за компанию.
type RepresentativeRole string

// Компания, за которую пользователь может подавать заявки.
type Membership struct {
	CompanyID uuid.UUID
	INN       string
	ShortName string
	Kind      string
	Simulated bool
	Role      RepresentativeRole
}

// Пользователь с его компаниями, как его видит кабинет.
type Me struct {
	User        User
	Memberships []Membership
}

// Серверная сессия, токен в cookie несёт только её ID.
type Session struct {
	ID     string
	UserID uuid.UUID
	Role   Role
}
