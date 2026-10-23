package usecase_test

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const (
	coffeeINN    = "0000100012"
	logisticsINN = "000020001010"
)

var draft = domain.Draft{
	Role:       "director",
	Product:    domain.WorkingCapital,
	Purpose:    domain.GoalPurchase,
	AmountKop:  250000000,
	TermMonths: 24,
	BKIConsent: true,
	PDConsent:  true,
	Comment:    "закупка к зиме",
}

func (e *appEnv) role(login, inn string) domain.RepresentativeRole {
	var r domain.RepresentativeRole
	err := e.pool.QueryRow(e.t.Context(), `
		SELECT m.representative_role FROM company_members m
		JOIN users u ON u.id = m.user_id JOIN companies c ON c.id = m.company_id
		WHERE u.login = $1 AND c.inn = $2`, login, inn).Scan(&r)
	require.NoError(e.t, err)
	return r
}

func (e *appEnv) count() int {
	var n int
	require.NoError(e.t, e.pool.QueryRow(e.t.Context(), "SELECT count(*) FROM applications").Scan(&n))
	return n
}

func TestCreate(t *testing.T) {
	e := newAppEnv(t)
	logistics := e.user("logistics")
	e.egrul.c = domain.Company{
		Kind: domain.KindUL, OGRN: "1000000600015", KPP: "000101001",
		ShortName: "ООО «Новая точка»", FullName: "Общество с ограниченной ответственностью «Новая точка»",
		Address: "г. Тверь, ул. Советская, д. 1", RegisteredAt: time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		Okved: domain.Okved{Code: "56.10"}, Simulated: true,
	}
	first, err := e.uc.Create(t.Context(), e.client, "coffee-key-1", coffeeINN, draft)
	require.NoError(t, err)

	other := draft
	other.AmountKop = 300000000
	proxy := draft
	proxy.Role = "proxy"
	ip := draft
	ip.Role = "owner_ip"

	tests := []struct {
		name     string
		actor    domain.Actor
		key, inn string
		d        domain.Draft
		egrulErr error
		same     bool
		err      error
		calls    int
	}{
		{name: "повтор с тем же ключом", actor: e.client, key: "coffee-key-1", inn: coffeeINN, d: draft, same: true},
		{name: "тот же ключ, другая сумма", actor: e.client, key: "coffee-key-1", inn: coffeeINN, d: other,
			err: domain.ErrKeyReused},
		{name: "тот же ключ, другая роль", actor: e.client, key: "coffee-key-1", inn: coffeeINN, d: proxy,
			err: domain.ErrKeyReused},
		{name: "новый ключ", actor: e.client, key: "coffee-key-2", inn: coffeeINN, d: draft},
		{name: "ключи у каждого свои", actor: logistics, key: "coffee-key-1", inn: logisticsINN, d: ip},
		{name: "чужая компания", actor: e.client, key: "coffee-key-3", inn: logisticsINN, d: draft,
			err: domain.ErrForbidden},
		{name: "новая компания из ЕГРЮЛ", actor: e.client, key: "coffee-key-4", inn: "0000600015", d: proxy, calls: 1},
		{name: "в ЕГРЮЛ нет", actor: e.client, key: "coffee-key-5", inn: "0000600022", d: draft,
			egrulErr: domain.ErrNotFound, err: domain.ErrInvalid, calls: 1},
		{name: "ЕГРЮЛ не отвечает", actor: e.client, key: "coffee-key-6", inn: "0000600022", d: draft,
			egrulErr: domain.ErrUnavailable, err: domain.ErrUnavailable, calls: 1},
		{name: "контрольная цифра", actor: e.client, key: "coffee-key-7", inn: "0000600016", d: draft,
			err: domain.ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e.egrul.calls, e.egrul.err = 0, tt.egrulErr
			before := e.count()

			app, cerr := e.uc.Create(t.Context(), tt.actor, tt.key, tt.inn, tt.d)
			require.Equal(t, tt.calls, e.egrul.calls)
			if tt.err != nil {
				require.ErrorIs(t, cerr, tt.err)
				require.Equal(t, before, e.count())
				require.Equal(t, domain.RepresentativeRole("director"), e.role("coffee", coffeeINN), "роль откатилась")
				return
			}
			require.NoError(t, cerr)
			if tt.same {
				require.Equal(t, first, app)
				require.Equal(t, before, e.count())
				return
			}
			require.NotEqual(t, first.ID, app.ID)
			require.Equal(t, before+1, e.count())
			require.Equal(t, []any{domain.StatusDraft, 1, tt.inn, tt.d},
				[]any{app.Status, app.Version, app.Company.INN, app.Draft})
		})
	}

	require.Equal(t, "ООО «Тёплое зерно»", first.Company.ShortName)
	created, err := e.uc.Get(t.Context(), e.client, e.lastID())
	require.NoError(t, err)
	require.Equal(
		t,
		domain.Company{INN: "0000600015", Kind: domain.KindUL, ShortName: "ООО «Новая точка»", Simulated: true},
		created.Company,
	)
}

func (e *appEnv) lastID() uuid.UUID {
	var id uuid.UUID
	err := e.pool.QueryRow(e.t.Context(), `
		SELECT a.id FROM applications a JOIN companies c ON c.id = a.company_id WHERE c.inn = '0000600015'`).Scan(&id)
	require.NoError(e.t, err)
	return id
}

func TestCreateRace(t *testing.T) {
	e := newAppEnv(t)

	// двойной клик: оба запроса с одним ключом, заявка одна
	ids := make([]uuid.UUID, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			app, err := e.uc.Create(t.Context(), e.client, "double-click", coffeeINN, draft)
			ids[i], errs[i] = app.ID, err
		})
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, ids[0], ids[1])
	require.Equal(t, 1, e.count())
}

func TestOwnership(t *testing.T) {
	e := newAppEnv(t)
	logistics, manager := e.user("logistics"), e.user("manager")
	app, err := e.uc.Create(t.Context(), e.client, "coffee-key-1", coffeeINN, draft)
	require.NoError(t, err)

	tests := []struct {
		name  string
		actor domain.Actor
		err   error
	}{
		{"автор", e.client, nil},
		{"другой клиент", logistics, domain.ErrNotFound},
		{"сотрудник банка до подачи не видит", manager, domain.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gerr := e.uc.Get(t.Context(), tt.actor, app.ID)
			require.ErrorIs(t, gerr, tt.err)

			list, total, lerr := e.uc.List(t.Context(), tt.actor, domain.ApplicationFilter{Limit: 20})
			require.NoError(t, lerr)
			require.Equal(t, len(list), total)
			require.Equal(t, tt.err == nil, total == 1)
		})
	}

	// чужой не может ни поправить, ни отменить, и не узнаёт, что заявка есть
	_, err = e.uc.Edit(t.Context(), logistics, app.ID, 1, func(d *domain.Draft) { d.AmountKop = 10000000 })
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = e.uc.Cancel(t.Context(), logistics, app.ID, "")
	require.ErrorIs(t, err, domain.ErrNotFound)
	status, version, _ := e.state(app.ID)
	require.Equal(t, []any{domain.StatusDraft, 1}, []any{status, version})
}

func TestEdit(t *testing.T) {
	e := newAppEnv(t)

	tests := []struct {
		name    string
		from    domain.Status
		version int
		edit    func(*domain.Draft)
		want    domain.Draft
		err     error
	}{
		{
			name: "сумма, срок и роль", from: domain.StatusDraft, version: 1,
			edit: func(d *domain.Draft) { d.AmountKop, d.TermMonths, d.Role = 100000000, 12, "proxy" },
			want: domain.Draft{
				Role: "proxy", Product: domain.WorkingCapital, Purpose: domain.GoalPurchase,
				AmountKop: 100000000, TermMonths: 12, BKIConsent: true, PDConsent: true, Comment: "закупка к зиме",
			},
		},
		{
			name: "оборудование и снятое согласие", from: domain.StatusDraft, version: 1,
			edit: func(d *domain.Draft) {
				d.Product, d.Purpose, d.EquipmentCostKop, d.BKIConsent, d.Comment =
					domain.Equipment, domain.GoalEquipment, 670000000, false, ""
			},
			want: domain.Draft{
				Role: "director", Product: domain.Equipment, Purpose: domain.GoalEquipment,
				AmountKop: 250000000, TermMonths: 24, EquipmentCostKop: 670000000, PDConsent: true,
			},
		},
		{name: "карточка устарела", from: domain.StatusDraft, version: 7, err: domain.ErrConflict},
		{name: "уже подана", from: domain.StatusSubmitted, version: 1, err: domain.ErrIllegalTransition},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var id uuid.UUID
			if tt.from == domain.StatusDraft {
				app, err := e.uc.Create(t.Context(), e.client, "edit-key-"+string(rune('a'+i)), coffeeINN, draft)
				require.NoError(t, err)
				id = app.ID
			} else {
				id = e.app(tt.from)
			}
			if tt.edit == nil {
				tt.edit = func(d *domain.Draft) { d.AmountKop = 10000000 }
			}

			app, err := e.uc.Edit(t.Context(), e.client, id, tt.version, tt.edit)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				_, version, _ := e.state(id)
				require.Equal(t, 1, version)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []any{2, tt.want}, []any{app.Version, app.Draft})
			require.Equal(t, tt.want.Role, e.role("coffee", coffeeINN))
		})
	}
}

func TestCancel(t *testing.T) {
	e := newAppEnv(t)
	app, err := e.uc.Create(t.Context(), e.client, "cancel-key-1", coffeeINN, draft)
	require.NoError(t, err)

	got, err := e.uc.Cancel(t.Context(), e.client, app.ID, "нашли деньги у поставщика")
	require.NoError(t, err)
	require.Equal(t, []any{domain.StatusCancelledByClient, 2}, []any{got.Status, got.Version})
	var comment string
	err = e.pool.QueryRow(t.Context(), "SELECT comment FROM application_events WHERE application_id = $1 AND actor_id = $2",
		app.ID, e.client.ID).
		Scan(&comment)
	require.NoError(t, err)
	require.Equal(t, "нашли деньги у поставщика", comment)

	// повтор и поданная заявка: отмена после подачи пойдёт через процесс, не сюда
	for _, id := range []uuid.UUID{app.ID, e.app(domain.StatusSubmitted)} {
		_, err = e.uc.Cancel(t.Context(), e.client, id, "")
		require.ErrorIs(t, err, domain.ErrIllegalTransition)
	}
}

func TestList(t *testing.T) {
	e := newAppEnv(t)
	ids := make([]uuid.UUID, 3)
	for i := range ids {
		app, err := e.uc.Create(t.Context(), e.client, "list-key-"+string(rune('a'+i)), coffeeINN, draft)
		require.NoError(t, err)
		ids[i] = app.ID
	}
	_, err := e.uc.Cancel(t.Context(), e.client, ids[0], "")
	require.NoError(t, err)

	tests := []struct {
		name  string
		f     domain.ApplicationFilter
		want  []uuid.UUID
		total int
	}{
		{"все, новые сверху", domain.ApplicationFilter{Limit: 20}, []uuid.UUID{ids[2], ids[1], ids[0]}, 3},
		{"страница", domain.ApplicationFilter{Limit: 1, Offset: 1}, []uuid.UUID{ids[1]}, 3},
		{"по статусу", domain.ApplicationFilter{Limit: 20, Statuses: []domain.Status{domain.StatusCancelledByClient}},
			[]uuid.UUID{ids[0]}, 1},
		{"по продукту", domain.ApplicationFilter{Limit: 20, Product: domain.Overdraft}, []uuid.UUID{}, 0},
		{
			"по классу, скоринга ещё не было",
			domain.ApplicationFilter{Limit: 20, Grades: []string{"A"}},
			[]uuid.UUID{},
			0,
		},
		// чужой Owner из фильтра не проходит, его ставит usecase
		{"owner из запроса не в счёт", domain.ApplicationFilter{Limit: 20, Owner: &uuid.UUID{}},
			[]uuid.UUID{ids[2], ids[1], ids[0]}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, total, err := e.uc.List(t.Context(), e.client, tt.f)
			require.NoError(t, err)
			got := []uuid.UUID{}
			for _, a := range list {
				got = append(got, a.ID)
			}
			require.Equal(t, []any{tt.want, tt.total}, []any{got, total})
		})
	}
}
