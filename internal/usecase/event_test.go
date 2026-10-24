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

func TestEvents(t *testing.T) {
	e := newAppEnv(t)
	events := repo.NewEvent(zap.NewNop(), &postgres.Client{Pool: e.pool})
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error)
	go func() { done <- events.Run(ctx) }()
	t.Cleanup(func() { stop(); <-done })

	// уведомление до LISTEN никому не придёт, ждём, пока слушатель подпишется
	require.Eventually(t, func() bool {
		var n int
		err := e.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND query = 'LISTEN applications'`).Scan(&n)
		return err == nil && n == 1
	}, 10*time.Second, 50*time.Millisecond)

	uc := usecase.NewEvent(events, repo.NewUser(db.New(e.pool)))
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
