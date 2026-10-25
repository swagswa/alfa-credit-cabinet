package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/migrations"
	"github.com/swagswa/alfa-credit-cabinet/pkg/testenv"
)

func TestNotifications(t *testing.T) {
	pool := testenv.Postgres(t, migrations.FS)
	ctx := t.Context()
	var app uuid.UUID
	require.NoError(
		t,
		pool.QueryRow(ctx, "SELECT application_id FROM documents WHERE id = $1", upload(t, pool)).Scan(&app),
	)
	coffee := uuid.MustParse("01930000-0000-7000-8000-000000000001")
	logistics := uuid.MustParse("01930000-0000-7000-8000-000000000002")
	// второй представитель и сотрудник банка в той же компании: сотруднику писать нельзя
	_, err := pool.Exec(ctx, `INSERT INTO company_members (user_id, company_id, representative_role) VALUES
		($1, '01930000-0000-7000-8000-000000000101', 'proxy'),
		('01930000-0000-7000-8000-000000000011', '01930000-0000-7000-8000-000000000101', 'proxy')`, logistics)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(ctx, "LISTEN applications")
	require.NoError(t, err)

	r := repo.NewNotification(pool)
	require.NoError(t, r.Notify(ctx, app, "approved", domain.Phrase{Title: "Одобрено", Text: "Кредит"}))

	var n int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&n))
	require.Equal(t, 2, n, "только клиентам компании")

	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ev, err := conn.Conn().WaitForNotification(wait)
	require.NoError(t, err)
	require.Contains(t, ev.Payload, `"notification_user"`)

	list, err := r.List(ctx, coffee, true, 50)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, domain.Notification{
		ID:            list[0].ID,
		ApplicationID: app,
		Kind:          "approved",
		Title:         "Одобрено",
		Text:          "Кредит",
		CreatedAt:     list[0].CreatedAt,
	}, list[0])

	require.ErrorIs(t, r.Read(ctx, logistics, list[0].ID), domain.ErrNotFound, "чужое уведомление")
	require.NoError(t, r.Read(ctx, coffee, list[0].ID))
	require.NoError(t, r.Read(ctx, coffee, list[0].ID), "повтор не ошибка")
	list, err = r.List(ctx, coffee, true, 50)
	require.NoError(t, err)
	require.Empty(t, list)

	require.NoError(t, r.ReadAll(ctx, logistics))
	list, err = r.List(ctx, logistics, false, 50)
	require.NoError(t, err)
	require.NotNil(t, list[0].ReadAt)

	// выдача берёт счёт из ответа данных банка по заявке
	_, err = pool.Exec(ctx, `INSERT INTO check_results (application_id, source, status, payload)
		VALUES ($1, 'internal', 'ok', '{"account": "40702810000000100012"}')`, app)
	require.NoError(t, err)
	require.NoError(t, repo.NewSigning(pool).Payout(ctx, app, 100000000))
	v, err := r.Vars(ctx, app)
	require.NoError(t, err)
	require.Equal(t, domain.NoticeVars{Number: v.Number, AmountKop: 100000000, Account: "40702810000000100012"}, v)
	require.Positive(t, v.Number)
}
