package usecase_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// queue раскладывает по заявке кофейни и айтишников в разных статусах, порядок создания
// совпадает с порядком в списке: c1 самая старая.
func (e *appEnv) queue() map[string]uuid.UUID {
	rows, err := e.pool.Query(e.t.Context(), `
		INSERT INTO applications (company_id, created_by, idempotency_key, status, product, purpose,
			amount_kop, term_months, bki_consent_at, pd_consent_at, submitted_at, score, grade, due_at, created_at)
		SELECT m.company_id, m.user_id, q.key, q.status, q.product, 'purchase', q.amount, 12, now(), now(),
			CASE WHEN q.status <> 'draft' THEN now() END, q.score, q.grade,
			now() + q.due * interval '1 hour', now() - q.age * interval '1 minute'
		FROM (VALUES
			('c1', 'coffee', 'manual_review', 'working_capital', 300000000, 61, 'C', 2, 50),
			('c2', 'coffee', 'draft', 'overdraft', 50000000, NULL, NULL, NULL, 40),
			('i1', 'it', 'manual_review', 'working_capital', 100000000, 48, 'D', 1, 30),
			('i2', 'it', 'submitted', 'equipment', 700000000, NULL, NULL, NULL, 20),
			('i3', 'it', 'rejected', 'working_capital', 200000000, 87, 'A', NULL, 10)
		) AS q (key, login, status, product, amount, score, grade, due, age)
		JOIN users u ON u.login = q.login
		JOIN company_members m ON m.user_id = u.id
		RETURNING idempotency_key, id`)
	require.NoError(e.t, err)
	ids := map[string]uuid.UUID{}
	for rows.Next() {
		var (
			key string
			id  uuid.UUID
		)
		require.NoError(e.t, rows.Scan(&key, &id))
		ids[key] = id
	}
	require.NoError(e.t, rows.Err())
	return ids
}

func TestQueue(t *testing.T) {
	e := newAppEnv(t)
	ids := e.queue()
	manager, underwriter, admin := e.user("manager"), e.user("underwriter"), e.user("admin")

	tests := []struct {
		name  string
		actor domain.Actor
		f     domain.ApplicationFilter
		want  []string
		total int
	}{
		{"менеджер без черновиков", manager, domain.ApplicationFilter{}, []string{"i3", "i2", "i1", "c1"}, 4},
		{"андеррайтер тоже", underwriter, domain.ApplicationFilter{}, []string{"i3", "i2", "i1", "c1"}, 4},
		{"клиент только свои", e.client, domain.ApplicationFilter{}, []string{"c2", "c1"}, 2},
		{"ручное рассмотрение", admin, domain.ApplicationFilter{Statuses: []domain.Status{domain.StatusManualReview}},
			[]string{"i1", "c1"}, 2},
		{"два статуса", manager, domain.ApplicationFilter{
			Statuses: []domain.Status{domain.StatusSubmitted, domain.StatusRejected},
		}, []string{"i3", "i2"}, 2},
		{"класс риска", manager, domain.ApplicationFilter{Grades: []string{"C", "D"}}, []string{"i1", "c1"}, 2},
		{"продукт", manager, domain.ApplicationFilter{Product: domain.WorkingCapital}, []string{"i3", "i1", "c1"}, 3},
		{"фильтр клиенту не расширяет", e.client, domain.ApplicationFilter{Product: domain.WorkingCapital},
			[]string{"c1"}, 1},
		{"старые сверху", manager, domain.ApplicationFilter{Sort: "created_at"},
			[]string{"c1", "i1", "i2", "i3"}, 4},
		{"по сумме", manager, domain.ApplicationFilter{Sort: "-amount_kop"},
			[]string{"i2", "c1", "i3", "i1"}, 4},
		{"по сумме вверх", manager, domain.ApplicationFilter{Sort: "amount_kop"},
			[]string{"i1", "i3", "c1", "i2"}, 4},
		// без срока в конце, среди них новые сверху
		{"горящие сроки", manager, domain.ApplicationFilter{Sort: "due_at"},
			[]string{"i1", "c1", "i3", "i2"}, 4},
		{"рискованные сверху", manager, domain.ApplicationFilter{Sort: "score"},
			[]string{"i1", "c1", "i3", "i2"}, 4},
		{"надёжные сверху", manager, domain.ApplicationFilter{Sort: "-score"},
			[]string{"i3", "c1", "i1", "i2"}, 4},
		{"страница", manager, domain.ApplicationFilter{Sort: "created_at", Limit: 2, Offset: 2},
			[]string{"i2", "i3"}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.f.Limit == 0 {
				tt.f.Limit = 20
			}
			list, total, err := e.uc.List(t.Context(), tt.actor, tt.f)
			require.NoError(t, err)
			got := []uuid.UUID{}
			for _, a := range list {
				got = append(got, a.ID)
			}
			want := []uuid.UUID{}
			for _, k := range tt.want {
				want = append(want, ids[k])
			}
			require.Equal(t, want, got)
			require.Equal(t, tt.total, total)
		})
	}
}

func TestStaffCard(t *testing.T) {
	e := newAppEnv(t)
	ids := e.queue()
	manager := e.user("manager")
	_, err := e.pool.Exec(t.Context(), `
		INSERT INTO check_results (application_id, source, status, payload, error) VALUES
			($1, 'bki', 'unavailable', NULL, 'таймаут'),
			($1, 'egrul', 'ok', '{"status": "active"}', NULL),
			($1, 'bki', 'ok', '{"summary": {"overdue_total": 0}}', NULL)`, ids["i1"])
	require.NoError(t, err)

	app, err := e.uc.Get(t.Context(), manager, ids["i1"])
	require.NoError(t, err)
	require.Equal(t, "ООО «Кодовая база»", app.Company.ShortName)
	require.Len(t, app.Checks, 2)
	// повтор после таймаута перекрывает прошлый результат
	bki, egrul := app.Checks[0], app.Checks[1]
	require.Equal(t, []any{domain.SourceBKI, domain.CheckOK, `{"summary": {"overdue_total": 0}}`, ""},
		[]any{bki.Source, bki.Status, string(bki.Payload), bki.Error})
	require.Equal(t, []any{domain.SourceEGRUL, domain.CheckOK, `{"status": "active"}`},
		[]any{egrul.Source, egrul.Status, string(egrul.Payload)})
	require.False(t, bki.At.IsZero())

	// клиенту проверки не показываем, чужую заявку он не находит
	own, err := e.uc.Get(t.Context(), e.user("it"), ids["i1"])
	require.NoError(t, err)
	require.Empty(t, own.Checks)
	_, err = e.uc.Get(t.Context(), e.client, ids["i1"])
	require.ErrorIs(t, err, domain.ErrNotFound)

	// черновик до подачи банку не виден, ни карточка, ни журнал
	_, err = e.uc.Get(t.Context(), manager, ids["c2"])
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = e.uc.History(t.Context(), manager, ids["c2"])
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestHistory(t *testing.T) {
	e := newAppEnv(t)
	underwriter := e.user("underwriter")
	id := e.app(domain.StatusSubmitted)
	moves := []domain.Move{
		{To: domain.StatusChecking, Actor: system},
		{To: domain.StatusScoring, Actor: system},
		{To: domain.StatusManualReview, Actor: system, Codes: []string{"R_MANUAL_RISK"}, Comment: "SF_DOC_MISMATCH"},
		{To: domain.StatusCounterOffer, Actor: underwriter, Codes: []string{"R_AMOUNT_REDUCED"},
			Comment: "оборот не тянет полную сумму"},
		{To: domain.StatusCancelledByClient, Actor: e.client, Comment: "нашли деньги сами"},
	}
	for _, m := range moves {
		require.NoError(t, e.uc.Transition(t.Context(), id, m))
	}

	tests := []struct {
		name     string
		actor    domain.Actor
		comments []string
		err      error
	}{
		{"сотрудник видит всё", e.user("manager"), []string{
			"", "", "SF_DOC_MISMATCH", "оборот не тянет полную сумму", "нашли деньги сами",
		}, nil},
		{"клиенту только свои комментарии", e.client, []string{"", "", "", "", "нашли деньги сами"}, nil},
		{"чужой клиент", e.user("it"), nil, domain.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.uc.History(t.Context(), tt.actor, id)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, len(moves))
			comments := []string{}
			for i, ev := range got {
				require.Equal(t, moves[i].To, ev.To)
				require.Equal(t, moves[i].Actor.Role, ev.ActorRole)
				require.Equal(t, append([]string{}, moves[i].Codes...), ev.Codes)
				if i > 0 {
					require.Greater(t, ev.ID, got[i-1].ID)
					require.False(t, ev.At.Before(got[i-1].At))
				}
				comments = append(comments, ev.Comment)
			}
			require.Equal(t, tt.comments, comments)
			require.Equal(t, []any{domain.StatusSubmitted, "transition", ""},
				[]any{got[0].From, got[0].Kind, got[0].ActorName})
			require.Equal(t, "Никитин Павел Алексеевич", got[3].ActorName)
		})
	}

	// у черновика журнал пустой, но это не 404
	draftID := e.app(domain.StatusDraft)
	got, err := e.uc.History(t.Context(), e.client, draftID)
	require.NoError(t, err)
	require.Empty(t, got)
}
