package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

// listen запускает слушателя LISTEN и ждёт подписки: уведомление до неё никому не придёт
func listen(t *testing.T, e *appEnv) usecase.EventUseCase {
	events := repo.NewEvent(zap.NewNop(), &postgres.Client{Pool: e.pool})
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error)
	go func() { done <- events.Run(ctx) }()
	t.Cleanup(func() { stop(); <-done })

	require.Eventually(t, func() bool {
		var n int
		err := e.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND query = 'LISTEN applications'`).Scan(&n)
		return err == nil && n == 1
	}, 10*time.Second, 50*time.Millisecond)
	return usecase.NewEvent(events, repo.NewUser(db.New(e.pool)))
}

func TestEvents(t *testing.T) {
	e := newAppEnv(t)
	uc := listen(t, e)
	id, draft := e.app(domain.StatusSubmitted), e.app(domain.StatusDraft)

	tests := []struct {
		name, login string
		app         uuid.UUID
		want, draft bool
	}{
		{"своя заявка", "coffee", uuid.Nil, true, true},
		{"своя по фильтру", "coffee", id, true, false},
		{"чужой клиент", "logistics", uuid.Nil, false, false},
		{"менеджер без черновиков", "manager", uuid.Nil, true, false},
		{"андеррайтер тоже", "underwriter", uuid.Nil, true, false},
		{"фильтр по другой заявке", "manager", uuid.New(), false, false},
	}
	subs := make([]<-chan domain.Event, len(tests))
	for i, tt := range tests {
		a := e.user(tt.login)
		ch, cancel, err := uc.Subscribe(t.Context(), domain.Session{ID: "s", UserID: a.ID, Role: a.Role}, tt.app)
		require.NoError(t, err)
		t.Cleanup(cancel)
		subs[i] = ch
	}

	// документ к черновику и его отмена приходят клиенту первыми, банку ни то ни другое:
	// отменённый черновик тоже ни разу не подавали
	_, err := e.pool.Exec(t.Context(), `
		INSERT INTO documents (application_id, kind, file_name, content_type, size_bytes, sha256, content, uploaded_by)
		VALUES ($1, 'passport', 'p.pdf', 'application/pdf', 1, sha256('x'), 'x', $2)`, draft, e.client.ID)
	require.NoError(t, err)
	cancel := domain.Move{To: domain.StatusCancelledByClient, Actor: e.client}
	require.NoError(t, e.uc.Transition(t.Context(), draft, cancel))
	require.NoError(t, e.uc.Transition(t.Context(), id, domain.Move{To: domain.StatusChecking, Actor: system}))

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range []domain.Status{domain.StatusDraft, domain.StatusCancelledByClient} {
				if !tt.draft {
					break
				}
				select {
				case ev := <-subs[i]:
					require.Equal(t, []any{draft, want}, []any{ev.ApplicationID, ev.Status})
				case <-time.After(time.Second):
					require.Fail(t, "событие черновика не пришло", want)
				}
			}
			select {
			case ev := <-subs[i]:
				require.True(t, tt.want, "лишнее событие %+v", ev)
				require.Equal(t, id, ev.ApplicationID)
				require.Equal(t, domain.StatusChecking, ev.Status)
				require.Equal(t, 2, ev.Version)
			case <-time.After(time.Second):
				require.False(t, tt.want, "событие не пришло")
			}
		})
	}
}

// уведомление в кабинете уходит в поток только адресату, даже другим клиентам компании нет
func TestEventsNotification(t *testing.T) {
	e := newAppEnv(t)
	uc := listen(t, e)
	id := e.app(domain.StatusSigning)
	// logistics на этот раз тоже представитель кофейни, но адресат только coffee
	_, err := e.pool.Exec(t.Context(), `INSERT INTO company_members (user_id, company_id, representative_role)
		SELECT u.id, m.company_id, 'proxy' FROM users u, company_members m
		WHERE u.login = 'logistics' AND m.user_id = $1`, e.client.ID)
	require.NoError(t, err)
	subs := map[string]<-chan domain.Event{}
	for _, login := range []string{"coffee", "logistics", "manager"} {
		a := e.user(login)
		ch, cancel, subErr := uc.Subscribe(t.Context(), domain.Session{ID: "s", UserID: a.ID, Role: a.Role}, uuid.Nil)
		require.NoError(t, subErr)
		t.Cleanup(cancel)
		subs[login] = ch
	}
	_, err = e.pool.Exec(t.Context(), `INSERT INTO notifications (user_id, application_id, kind, title, body)
		VALUES ($1, $2, 'signing_code', 'Код', 'Код 123456')`, e.client.ID, id)
	require.NoError(t, err)

	select {
	case ev := <-subs["coffee"]:
		require.Equal(t, []any{id, e.client.ID}, []any{ev.ApplicationID, ev.NotificationUser})
	case <-time.After(time.Second):
		require.Fail(t, "уведомление не пришло")
	}
	for _, login := range []string{"logistics", "manager"} {
		select {
		case ev := <-subs[login]:
			require.Fail(t, "чужое уведомление", "%s: %+v", login, ev)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
