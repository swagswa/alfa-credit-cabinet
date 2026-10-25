package repo_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestUserRepository(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(t.Context(), pool))
	r := repo.NewUser(db.New(pool))
	ctx := t.Context()

	u, hash, err := r.Credentials(ctx, "coffee")
	require.NoError(t, err)
	require.Equal(t, domain.RoleClient, u.Role)
	require.NoError(t, bcrypt.CompareHashAndPassword(hash, []byte("demo12345")))

	_, _, err = r.Credentials(ctx, "nobody")
	require.ErrorIs(t, err, domain.ErrNotFound)

	ms, err := r.Memberships(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, ms, 1)
	require.Equal(t, "0000100012", ms[0].INN)
	require.Equal(t, domain.RepresentativeRole("director"), ms[0].Role)

	staff, _, err := r.Credentials(ctx, "underwriter")
	require.NoError(t, err)
	ms, err = r.Memberships(ctx, staff.ID)
	require.NoError(t, err)
	require.Empty(t, ms, "у сотрудника компаний нет")

	_, err = r.ByID(ctx, uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUserProfile(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	app := upload(t, pool)
	require.NoError(t, pool.QueryRow(ctx, "SELECT application_id FROM documents WHERE id = $1", app).Scan(&app))
	r := repo.NewUser(db.New(pool))
	coffee := uuid.MustParse("01930000-0000-7000-8000-000000000001")

	u, err := r.ByID(ctx, coffee)
	require.NoError(t, err)
	require.Equal(t, "+79000000001", u.Phone)
	require.Equal(t, "coffee@demo.alfa-hack.local", u.Email)

	cs, err := r.Companies(ctx, coffee)
	require.NoError(t, err)
	require.Len(t, cs, 1)
	require.Equal(t, domain.Okved{Code: "56.30", Name: "Подача напитков"}, cs[0].Okved)
	require.Equal(t, "000101001", cs[0].KPP)
	require.Equal(t, domain.RepresentativeRole("director"), cs[0].Role)

	loans, err := r.Loans(ctx, coffee)
	require.NoError(t, err)
	require.Empty(t, loans, "без выдачи заявка не кредит")

	terms := `{"product": "working_capital", "amount_kop": 100000000, "term_months": 12, "rate_bp": 1900,
		"payment_kop": 9216000}`
	_, err = pool.Exec(ctx, `INSERT INTO decisions (application_id, stage, outcome, score, grade, policy_version, terms)
		VALUES ($1, 'policy', 'approved', 80, 'A', 'v1', $2)`, app, terms)
	require.NoError(t, err)
	require.NoError(t, repo.NewSigning(pool).Payout(ctx, app, 100000000))
	require.NoError(t, repo.NewDocument(pool).AddContract(ctx, app,
		&domain.DocFile{FileName: "dogovor-1.pdf", Content: []byte("%PDF")}))

	loans, err = r.Loans(ctx, coffee)
	require.NoError(t, err)
	require.Len(t, loans, 1)
	require.Equal(t, app, loans[0].ApplicationID)
	require.Equal(t, int64(9216000), loans[0].Terms.PaymentKop)
	require.NotNil(t, loans[0].ContractID)

	// чужой кредит не виден
	loans, err = r.Loans(ctx, uuid.MustParse("01930000-0000-7000-8000-000000000002"))
	require.NoError(t, err)
	require.Empty(t, loans)
}
