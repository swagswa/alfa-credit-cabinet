package usecase_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

var system = domain.Actor{Role: domain.RoleSystem}

type appEnv struct {
	t      *testing.T
	pool   *pgxpool.Pool
	uc     usecase.ApplicationUseCase
	client domain.Actor
}

func newAppEnv(t *testing.T) *appEnv {
	pool := testenv.Postgres(t, migrations.FS)
	require.NoError(t, postgres.Exec(migrations.SeedDemo)(t.Context(), pool))
	e := &appEnv{
		t:    t,
		pool: pool,
		uc:   usecase.NewApplication(&postgres.Client{Pool: pool}, repo.NewApplication(db.New(pool))),
	}
	e.client = e.user("coffee")
	return e
}

func (e *appEnv) user(login string) domain.Actor {
	a := domain.Actor{}
	err := e.pool.QueryRow(e.t.Context(), "SELECT id, role FROM users WHERE login = $1", login).Scan(&a.ID, &a.Role)
	require.NoError(e.t, err)
	return a
}

// app заводит заявку кофейни сразу в нужном статусе, версия 1.
func (e *appEnv) app(status domain.Status) uuid.UUID {
	var id uuid.UUID
	err := e.pool.QueryRow(e.t.Context(), `
		INSERT INTO applications (company_id, created_by, idempotency_key, status, product, purpose,
			amount_kop, term_months, bki_consent_at, submitted_at,
			offer_product, offer_amount_kop, offer_term_months, offer_rate_bp)
		SELECT m.company_id, m.user_id, gen_random_uuid()::text, $1, 'working_capital', 'purchase',
			100000000, 12, now(), CASE WHEN $1 <> 'draft' THEN now() END,
			'working_capital', 100000000, 12, 1800
		FROM company_members m WHERE m.user_id = $2
		RETURNING id`, status, e.client.ID).Scan(&id)
	require.NoError(e.t, err)
	return id
}

func (e *appEnv) state(id uuid.UUID) (status domain.Status, version, events int) {
	err := e.pool.QueryRow(e.t.Context(), `
		SELECT status, version, (SELECT count(*) FROM application_events WHERE application_id = a.id)
		FROM applications a WHERE id = $1`, id).Scan(&status, &version, &events)
	require.NoError(e.t, err)
	return status, version, events
}

func TestTransition(t *testing.T) {
	e := newAppEnv(t)
	manager, underwriter := e.user("manager"), e.user("underwriter")

	tests := []struct {
		name      string
		from      domain.Status
		noConsent bool
		m         domain.Move
		err       error
	}{
		{"клиент подаёт черновик", domain.StatusDraft, false,
			domain.Move{To: domain.StatusSubmitted, Actor: e.client, Version: 1}, nil},
		{"черновик отменяется без согласия", domain.StatusDraft, true,
			domain.Move{To: domain.StatusCancelledByClient, Actor: e.client, Version: 1}, nil},
		{"система начинает проверку", domain.StatusSubmitted, false,
			domain.Move{To: domain.StatusChecking, Actor: system}, nil},
		{"андеррайтер предлагает другие условия", domain.StatusManualReview, false, domain.Move{
			To: domain.StatusCounterOffer, Actor: underwriter, Version: 1,
			Codes: []string{"R_AMOUNT_REDUCED"}, Comment: "оборот не тянет полную сумму",
		}, nil},
		{"менеджер за андеррайтера", domain.StatusManualReview, false,
			domain.Move{To: domain.StatusApproved, Actor: manager}, domain.ErrIllegalTransition},
		{"клиент за систему", domain.StatusChecking, false,
			domain.Move{To: domain.StatusScoring, Actor: e.client}, domain.ErrIllegalTransition},
		{"из финального", domain.StatusRejected, false,
			domain.Move{To: domain.StatusChecking, Actor: system}, domain.ErrIllegalTransition},
		{"карточка устарела", domain.StatusApproved, false,
			domain.Move{To: domain.StatusAccepted, Actor: e.client, Version: 7}, domain.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := e.app(tt.from)
			if tt.noConsent {
				_, err := e.pool.Exec(t.Context(), "UPDATE applications SET bki_consent_at = NULL WHERE id = $1", id)
				require.NoError(t, err)
			}
			err := e.uc.Transition(t.Context(), id, tt.m)

			status, version, events := e.state(id)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				require.Equal(t, []any{tt.from, 1, 0}, []any{status, version, events}, "ничего не должно поменяться")
				return
			}
			require.NoError(t, err)
			require.Equal(t, []any{tt.m.To, 2, 1}, []any{status, version, events})

			var (
				actorID *uuid.UUID
				role    domain.Role
				codes   []string
				comment *string
			)
			err = e.pool.QueryRow(t.Context(), `SELECT actor_id, actor_role, reason_codes, comment
				FROM application_events WHERE application_id = $1`, id).Scan(&actorID, &role, &codes, &comment)
			require.NoError(t, err)
			require.Equal(t, tt.m.Actor.Role, role)
			if tt.m.Actor.Role == domain.RoleSystem {
				require.Nil(t, actorID)
			} else {
				require.Equal(t, tt.m.Actor.ID, *actorID)
			}
			require.Equal(t, append([]string{}, tt.m.Codes...), codes)
			if tt.m.Comment != "" {
				require.Equal(t, tt.m.Comment, *comment)
			}
		})
	}
}

func TestTransitionRepeat(t *testing.T) {
	e := newAppEnv(t)
	id := e.app(domain.StatusSubmitted)
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	m := domain.Move{To: domain.StatusChecking, Actor: system, DueAt: &due}

	// повтор activity после коммита: статус уже целевой, второй раз ничего не пишем
	require.NoError(t, e.uc.Transition(t.Context(), id, m))
	require.NoError(t, e.uc.Transition(t.Context(), id, m))
	status, version, events := e.state(id)
	require.Equal(t, []any{domain.StatusChecking, 2, 1}, []any{status, version, events})

	var got time.Time
	require.NoError(t, e.pool.QueryRow(t.Context(), "SELECT due_at FROM applications WHERE id = $1", id).Scan(&got))
	require.True(t, due.Equal(got))

	err := e.uc.Transition(t.Context(), uuid.New(), m)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestTransitionRace(t *testing.T) {
	e := newAppEnv(t)
	id := e.app(domain.StatusApproved)

	// клиент жмёт "принять" и "отказаться" по одной и той же карточке
	targets := []domain.Status{domain.StatusAccepted, domain.StatusCancelledByClient}
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, to := range targets {
		wg.Go(func() {
			errs[i] = e.uc.Transition(t.Context(), id, domain.Move{To: to, Actor: e.client, Version: 1})
		})
	}
	wg.Wait()

	if errs[0] != nil {
		errs[0], errs[1] = errs[1], errs[0]
	}
	require.NoError(t, errs[0])
	require.ErrorIs(t, errs[1], domain.ErrConflict)
	_, version, events := e.state(id)
	require.Equal(t, []int{2, 1}, []int{version, events})
}

func TestTransitionNotify(t *testing.T) {
	e := newAppEnv(t)
	id := e.app(domain.StatusSubmitted)

	conn, err := e.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(t.Context(), "LISTEN applications")
	require.NoError(t, err)

	require.NoError(t, e.uc.Transition(t.Context(), id, domain.Move{To: domain.StatusChecking, Actor: system}))

	n, err := conn.Conn().WaitForNotification(t.Context())
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(n.Payload), &got))
	var company string
	err = e.pool.QueryRow(t.Context(), "SELECT company_id::text FROM applications WHERE id = $1", id).Scan(&company)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"application_id": id.String(), "company_id": company, "status": "checking", "version": float64(2),
	}, got)
}
