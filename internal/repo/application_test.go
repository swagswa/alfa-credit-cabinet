package repo_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestEscalate(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
		SELECT company_id, user_id, 'k', 'working_capital', 'purchase', 100000000, 12 FROM company_members LIMIT 1
		RETURNING id`).Scan(&id)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(ctx, "LISTEN applications")
	require.NoError(t, err)

	require.NoError(t, repo.NewApplication(db.New(pool)).Escalate(ctx, id))

	var kind, role, status string
	var version int
	err = pool.QueryRow(ctx, `SELECT e.kind, e.actor_role, a.status, a.version
		FROM application_events e JOIN applications a ON a.id = e.application_id WHERE a.id = $1`, id).
		Scan(&kind, &role, &status, &version)
	require.NoError(t, err)
	require.Equal(t, []any{"escalation", "system", "draft", 1}, []any{kind, role, status, version})

	n, err := conn.Conn().WaitForNotification(ctx)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(n.Payload), &got))
	require.Equal(t, id.String(), got["application_id"])
	require.Equal(t, "draft", got["status"])
}

func TestSaveDecision(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id, uw uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
		SELECT company_id, user_id, 'k', 'working_capital', 'purchase', 800000000, 12 FROM company_members LIMIT 1
		RETURNING id`).Scan(&id)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM users WHERE role = 'underwriter' LIMIT 1`).Scan(&uw))
	r := repo.NewApplication(db.New(pool))

	none, err := r.Decision(ctx, id)
	require.NoError(t, err)
	require.Nil(t, none)

	rec := domain.Terms{
		Product:    domain.WorkingCapital,
		AmountKop:  700000000,
		TermMonths: 12,
		RateBP:     2250,
		Guarantor:  true,
	}
	policy := domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusManualReview,
		Score:          &domain.Score{Points: 60, Grade: "C", PD: "до 5%", Version: "s1"},
		Recommendation: &rec,
		Reasons:        []domain.Reason{{Code: "R_MANUAL_AMOUNT", Text: "смотрит специалист"}},
		ScoringVersion: "s1", PolicyVersion: "p1",
	}
	require.NoError(
		t,
		r.SaveDecision(ctx, id, &domain.Move{Actor: domain.Actor{Role: domain.RoleSystem}, Decision: &policy}),
	)
	got, err := r.Decision(ctx, id)
	require.NoError(t, err)
	require.Equal(t, &policy, got)

	// андеррайтер режет сумму: условия становятся текущими, балл остаётся, в журнале override
	offer := rec
	offer.AmountKop, offer.PaymentKop = 600000000, 56200000
	uwd := policy
	uwd.Stage, uwd.Outcome, uwd.Terms, uwd.Override = domain.StageUnderwriter, domain.StatusCounterOffer, &offer, true
	m := domain.Move{Actor: domain.Actor{ID: uw, Role: domain.RoleUnderwriter}, Comment: "сезонность", Decision: &uwd}
	require.NoError(t, r.SaveDecision(ctx, id, &m))

	var amount, score int
	var events int
	err = pool.QueryRow(ctx, `SELECT a.offer_amount_kop, a.score,
		(SELECT count(*) FROM application_events e WHERE e.application_id = a.id AND e.kind = 'override')
		FROM applications a WHERE a.id = $1`, id).Scan(&amount, &score, &events)
	require.NoError(t, err)
	require.Equal(t, []int{600000000, 60, 1}, []int{amount, score, events})
	got, err = r.Decision(ctx, id)
	require.NoError(t, err)
	require.Equal(t, &uwd, got)
}

func TestInfoRequest(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(ctx, pool))
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (company_id, created_by, idempotency_key, product, purpose, amount_kop, term_months)
		SELECT company_id, user_id, 'k', 'working_capital', 'purchase', 100000000, 12 FROM company_members LIMIT 1
		RETURNING id`).Scan(&id)
	require.NoError(t, err)
	r := repo.NewApplication(db.New(pool))

	req := domain.InfoRequest{
		Documents: []domain.DocKind{domain.DocStatement, domain.DocFinancials},
		Reasons:   []domain.Reason{{Code: "SF_DOCS_MISSING", Text: "Не хватает документа: выписка"}},
	}
	require.NoError(t, r.AddInfoRequest(ctx, id, uuid.Nil, req))
	got, err := r.InfoRequest(ctx, id)
	require.NoError(t, err)
	require.Equal(t, &req, got)
	require.Error(t, r.AddInfoRequest(ctx, id, uuid.Nil, req), "открытый запрос на заявку один")
	require.Error(t, r.AddInfoRequest(ctx, id, uuid.Nil, domain.InfoRequest{Documents: []domain.DocKind{"selfie"}}))

	require.NoError(t, r.CloseInfoRequest(ctx, id))
	got, err = r.InfoRequest(ctx, id)
	require.NoError(t, err)
	require.Nil(t, got)
}
