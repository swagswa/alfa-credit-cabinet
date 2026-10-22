package domain

import "github.com/google/uuid"

type Role string

const (
	RoleClient      Role = "client"
	RoleManager     Role = "manager"
	RoleUnderwriter Role = "underwriter"
	RoleAdmin       Role = "admin"
)

type User struct {
	ID       uuid.UUID
	Login    string
	FullName string
	Role     Role
}

// RepresentativeRole говорит, на каком основании клиент действует за компанию.
type RepresentativeRole string

// Membership это компания, за которую пользователь может подавать заявки.
type Membership struct {
	CompanyID uuid.UUID
	INN       string
	ShortName string
	Kind      string
	Simulated bool
	Role      RepresentativeRole
}

// Me это пользователь вместе с его компаниями, как его видит кабинет.
type Me struct {
	User        User
	Memberships []Membership
}

// Session это серверная сессия входа, токен в cookie несёт только её ID.
type Session struct {
	ID     string
	UserID uuid.UUID
	Role   Role
}
