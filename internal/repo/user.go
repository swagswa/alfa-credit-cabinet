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
	return domain.User{ID: row.ID, Login: row.Login, FullName: row.FullName, Role: domain.Role(row.Role)}, nil
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

// notFound переводит отсутствие строки в доменную ошибку, остальное оборачивает.
func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}
