package usecase_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestSubmit(t *testing.T) {
	noConsent := draft
	noConsent.BKIConsent = false
	equipment := draft
	equipment.Product, equipment.Purpose, equipment.PDConsent = domain.Equipment, domain.GoalEquipment, false

	tests := []struct {
		name   string
		d      domain.Draft
		fields []string
	}{
		{"без согласия на бки", noConsent, []string{"bki_consent"}},
		{"оборудование без стоимости и согласия на пдн", equipment, []string{"equipment_cost_kop", "pd_consent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAppEnv(t)
			app, err := e.uc.Create(t.Context(), e.client, "submit-key", coffeeINN, tt.d)
			require.NoError(t, err)

			_, err = e.uc.Submit(t.Context(), e.client, app.ID)
			require.ErrorIs(t, err, domain.ErrIncomplete)
			var fe domain.FieldErrors
			require.ErrorAs(t, err, &fe)
			var fields []string
			for _, f := range fe {
				fields = append(fields, f.Field)
			}
			require.Equal(t, tt.fields, fields)
			status, version, _ := e.state(app.ID)
			require.Equal(t, []any{domain.StatusDraft, 1}, []any{status, version})
			require.Empty(t, e.wf.started)
		})
	}
}

func TestSubmitOnce(t *testing.T) {
	e := newAppEnv(t)
	app, err := e.uc.Create(t.Context(), e.client, "submit-key", coffeeINN, draft)
	require.NoError(t, err)

	got, err := e.uc.Submit(t.Context(), e.client, app.ID)
	require.NoError(t, err)
	require.Equal(t, []any{domain.StatusSubmitted, 2}, []any{got.Status, got.Version})
	require.NotNil(t, got.SubmittedAt)
	require.Equal(t, []any{"credit", app.ID.String(), "ApplicationWorkflow", app.ID}, e.wf.args)

	// повтор: процесс уже идёт, второй не заводим и ошибкой это не считаем
	again, err := e.uc.Submit(t.Context(), e.client, app.ID)
	require.NoError(t, err)
	require.Equal(t, got.Version, again.Version)
	require.Equal(t, []string{app.ID.String()}, e.wf.started)
	_, _, events := e.state(app.ID)
	require.Equal(t, 1, events)

	// воркер уже перевёл в checking, повтор всё так же ничего не ломает
	_, err = e.pool.Exec(t.Context(), "UPDATE applications SET status = 'checking' WHERE id = $1", app.ID)
	require.NoError(t, err)
	_, err = e.uc.Submit(t.Context(), e.client, app.ID)
	require.NoError(t, err)
	require.Len(t, e.wf.started, 1)
}

func TestSubmitTemporalDown(t *testing.T) {
	e := newAppEnv(t)
	app, err := e.uc.Create(t.Context(), e.client, "submit-key", coffeeINN, draft)
	require.NoError(t, err)

	// переход уже закоммичен, процесса нет: клиент получает 503 и жмёт ещё раз
	e.wf.err = errors.New("temporal down")
	_, err = e.uc.Submit(t.Context(), e.client, app.ID)
	require.ErrorIs(t, err, domain.ErrUnavailable)
	status, _, _ := e.state(app.ID)
	require.Equal(t, domain.StatusSubmitted, status)
	require.Empty(t, e.wf.started)

	e.wf.err = nil
	got, err := e.uc.Submit(t.Context(), e.client, app.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusSubmitted, got.Status)
	require.Equal(t, []string{app.ID.String()}, e.wf.started)
}

func TestSubmitDenied(t *testing.T) {
	e := newAppEnv(t)
	app, err := e.uc.Create(t.Context(), e.client, "submit-key", coffeeINN, draft)
	require.NoError(t, err)
	cancelled, err := e.uc.Create(t.Context(), e.client, "submit-key-2", coffeeINN, draft)
	require.NoError(t, err)
	_, err = e.uc.Cancel(t.Context(), e.client, cancelled.ID, "")
	require.NoError(t, err)

	tests := []struct {
		name  string
		actor domain.Actor
		app   domain.Application
		err   error
	}{
		{"чужая заявка", e.user("logistics"), app, domain.ErrNotFound},
		{"сотрудник банка за клиента", e.user("manager"), app, domain.ErrIllegalTransition},
		{"отменённый черновик", e.client, cancelled, domain.ErrIllegalTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, before := e.state(tt.app.ID)
			_, err := e.uc.Submit(t.Context(), tt.actor, tt.app.ID)
			require.ErrorIs(t, err, tt.err)
			_, _, events := e.state(tt.app.ID)
			require.Equal(t, before, events)
			require.Empty(t, e.wf.started)
		})
	}
}
