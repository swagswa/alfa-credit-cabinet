package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

type UserRepository interface {
	// Credentials отдаёт пользователя и bcrypt-хеш пароля по логину.
	Credentials(ctx context.Context, login string) (domain.User, []byte, error)
	ByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	Memberships(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error)
	// Companies отдаёт компании пользователя с реквизитами, без данных банка.
	Companies(ctx context.Context, userID uuid.UUID) ([]domain.ProfileCompany, error)
	// Loans: выданные кредиты компаний пользователя, новые сверху.
	Loans(ctx context.Context, userID uuid.UUID) ([]domain.Loan, error)
}

type userRepo struct {
	q *db.Queries
}

func NewUser(q *db.Queries) UserRepository {
	return &userRepo{q: q}
}

func (r *userRepo) Credentials(ctx context.Context, login string) (domain.User, []byte, error) {
	row, err := r.q.UserByLogin(ctx, login)
	if err != nil {
		return domain.User{}, nil, notFound(err, "user by login")
	}
	u := domain.User{ID: row.ID, Login: row.Login, FullName: row.FullName, Role: domain.Role(row.Role)}
	return u, []byte(row.PasswordHash), nil
}

func (r *userRepo) ByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := r.q.UserByID(ctx, id)
	if err != nil {
		return domain.User{}, notFound(err, "user "+id.String())
	}
	return domain.User{
		ID: row.ID, Login: row.Login, FullName: row.FullName, Role: domain.Role(row.Role),
		Phone: row.Phone, Email: row.Email,
	}, nil
}

func (r *userRepo) Memberships(ctx context.Context, userID uuid.UUID) ([]domain.Membership, error) {
	rows, err := r.q.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("memberships %s: %w", userID, err)
	}
	out := make([]domain.Membership, 0, len(rows))
	for _, m := range rows {
		out = append(out, domain.Membership{
			CompanyID: m.ID,
			INN:       m.Inn,
			ShortName: m.ShortName,
			Kind:      m.Kind,
			Simulated: m.Simulated,
			Role:      domain.RepresentativeRole(m.RepresentativeRole),
		})
	}
	return out, nil
}

func (r *userRepo) Companies(ctx context.Context, userID uuid.UUID) ([]domain.ProfileCompany, error) {
	rows, err := r.q.ProfileCompanies(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("profile companies %s: %w", userID, err)
	}
	out := make([]domain.ProfileCompany, 0, len(rows))
	for i := range rows {
		c := &rows[i]
		out = append(out, domain.ProfileCompany{
			ID:           c.ID,
			Role:         domain.RepresentativeRole(c.RepresentativeRole),
			INN:          c.Inn,
			OGRN:         c.Ogrn,
			KPP:          c.Kpp.String,
			Kind:         domain.CompanyKind(c.Kind),
			FullName:     c.FullName,
			ShortName:    c.ShortName,
			RegisteredAt: c.RegisteredAt.Time,
			Address:      c.Address,
			Okved:        domain.Okved{Code: c.Okved, Name: c.OkvedName},
			Simulated:    c.Simulated,
		})
	}
	return out, nil
}

func (r *userRepo) Loans(ctx context.Context, userID uuid.UUID) ([]domain.Loan, error) {
	rows, err := r.q.LoansByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("loans %s: %w", userID, err)
	}
	out := make([]domain.Loan, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		l := domain.Loan{
			ApplicationID: row.ID,
			CompanyID:     row.CompanyID,
			Number:        row.Number,
			Status:        domain.Status(row.Status),
			IssuedAt:      row.IssuedAt,
		}
		if err := unmarshal(row.Terms, &l.Terms); err != nil {
			return nil, err
		}
		if row.ContractID.Valid {
			id := uuid.UUID(row.ContractID.Bytes)
			l.ContractID = &id
		}
		out = append(out, l)
	}
	return out, nil
}

// notFound переводит отсутствие строки в доменную ошибку, остальное оборачивает.
func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}
