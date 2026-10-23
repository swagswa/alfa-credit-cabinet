package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

// Workflows стартует процессы и передаёт им действия людей, на стенде это temporalsvc.Client.
type Workflows interface {
	Start(ctx context.Context, queue, id, workflow string, args ...any) error
	Update(ctx context.Context, id, update string, arg any) error
}

type ApplicationUseCase interface {
	// Transition единственное место, где меняется статус заявки. Повтор того же
	// перехода ничего не делает, поэтому его можно звать из activity с ретраями.
	Transition(ctx context.Context, id uuid.UUID, m domain.Move) error
	// StartProcess запускает ApplicationWorkflow заявки, id процесса равен id заявки.
	// Второй запуск для той же заявки даёт ErrConflict.
	StartProcess(ctx context.Context, id uuid.UUID) error
	// Act передаёт действие человека процессу заявки и ждёт, пока тот его исполнит.
	// Действие не к месту: ErrIllegalTransition, устаревшая карточка: ErrConflict.
	Act(ctx context.Context, id uuid.UUID, update string, a domain.Action) error
	// Escalate отмечает в журнале, что SLA андеррайтера прошёл.
	Escalate(ctx context.Context, id uuid.UUID) error

	// Create заводит черновик по ИНН. Компании ещё нет у нас: берём её из ЕГРЮЛ,
	// и автор становится представителем. Чужую компанию так присвоить нельзя.
	// Повтор с тем же ключом отдаёт ту же заявку, с другим телом ErrKeyReused.
	Create(ctx context.Context, actor domain.Actor, key, inn string, d domain.Draft) (domain.Application, error)
	// Get отдаёт карточку. Сотруднику банка вместе с результатами проверок.
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error)
	List(ctx context.Context, actor domain.Actor, f domain.ApplicationFilter) ([]domain.Application, int, error)
	// History это журнал заявки. Клиент видит только свои комментарии, заметки банка и системы нет.
	History(ctx context.Context, actor domain.Actor, id uuid.UUID) ([]domain.AuditEntry, error)
	// Edit меняет анкету черновика той версии, которую видел клиент.
	Edit(ctx context.Context, actor domain.Actor, id uuid.UUID, version int, edit func(*domain.Draft)) (
		domain.Application, error)
	// Submit подаёт черновик и запускает его процесс. Повтор по уже поданной заявке
	// только дозапускает процесс, если в прошлый раз Temporal не ответил.
	Submit(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error)
	// Cancel отменяет черновик сам, поданную заявку через её процесс.
	Cancel(ctx context.Context, actor domain.Actor, id uuid.UUID, reason string) (domain.Application, error)
	// ProvideInfo это ответ клиента на запрос банка. Документы годятся подтверждённые
	// или те, что не распознаём (паспорт, доверенность). Выручку пишем до перехода.
	ProvideInfo(ctx context.Context, actor domain.Actor, id uuid.UUID, a Answer) error
}

// Answer это то, что клиент прислал на запрос уточнений.
type Answer struct {
	Documents  []uuid.UUID
	RevenueKop *int64
	Comment    string
}

type applicationUseCase struct {
	db        *postgres.Client
	apps      repo.ApplicationRepository
	companies CompanyUseCase
	workflows Workflows
	catalog   domain.Catalog
}

// NewApplication собирает usecase заявок. companies и catalog нужны только черновику и подаче,
// воркеру можно nil и пустой каталог.
func NewApplication(
	db *postgres.Client, apps repo.ApplicationRepository, companies CompanyUseCase, workflows Workflows,
	catalog domain.Catalog,
) ApplicationUseCase {
	return &applicationUseCase{db: db, apps: apps, companies: companies, workflows: workflows, catalog: catalog}
}

func (u *applicationUseCase) StartProcess(ctx context.Context, id uuid.UUID) error {
	err := u.workflows.Start(ctx, QueueCredit, id.String(), ApplicationWorkflowName, id)
	if errors.Is(err, temporalsvc.ErrAlreadyStarted) {
		return fmt.Errorf("%w: процесс заявки уже запущен", domain.ErrConflict)
	}
	return err
}

func (u *applicationUseCase) Act(ctx context.Context, id uuid.UUID, update string, a domain.Action) error {
	// сотрудник видит все заявки, клиент только свои: чужая для него не существует
	if a.Actor.Role == domain.RoleClient {
		if _, err := u.apps.Card(ctx, id, &a.Actor.ID); err != nil {
			return err
		}
	}
	return fromProcess(u.workflows.Update(ctx, id.String(), update, a))
}

func (u *applicationUseCase) Escalate(ctx context.Context, id uuid.UUID) error {
	return u.apps.Escalate(ctx, id)
}

func (u *applicationUseCase) Transition(ctx context.Context, id uuid.UUID, m domain.Move) error {
	return u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		apps := u.apps.WithTx(tx)
		app, err := apps.Lock(ctx, id)
		if err != nil {
			return err
		}
		// идемпотентность раньше версии: повтор после коммита видит уже новую версию
		switch {
		case app.Status == m.To:
			return nil
		case m.Version != 0 && m.Version != app.Version:
			return fmt.Errorf("%w: заявка уже изменилась, обновите карточку", domain.ErrConflict)
		case !domain.CanTransition(app.Status, m.To, m.Actor.Role):
			return fmt.Errorf("%w: из %s в %s ролью %s", domain.ErrIllegalTransition, app.Status, m.To, m.Actor.Role)
		}
		if err := apps.Move(ctx, app, m); err != nil {
			return err
		}
		if m.To == domain.StatusSubmitted {
			return apps.AddBKIConsent(ctx, id)
		}
		return nil
	})
}

func (u *applicationUseCase) Create(
	ctx context.Context, actor domain.Actor, key, inn string, d domain.Draft,
) (domain.Application, error) {
	hash := sha256.Sum256(fmt.Appendf(nil, "%s %+v", inn, d))
	companyID, egrul, err := u.company(ctx, actor, inn)
	if err != nil {
		return domain.Application{}, err
	}

	var id uuid.UUID
	err = u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		apps := u.apps.WithTx(tx)
		if egrul != nil {
			if companyID, err = apps.AddCompany(ctx, *egrul); err != nil {
				return err
			}
		}
		err = apps.SetMember(ctx, actor.ID, companyID, d.Role)
		if err != nil {
			return err
		}
		var saved []byte
		id, saved, err = apps.Create(ctx, actor.ID, key, hash[:], domain.Application{CompanyID: companyID, Draft: d})
		if err != nil {
			return err
		}
		// откат заодно вернёт роль, которую успели поменять выше
		if !bytes.Equal(saved, hash[:]) {
			return fmt.Errorf("%w: с этим ключом уже создана другая заявка", domain.ErrKeyReused)
		}
		return nil
	})
	if err != nil {
		return domain.Application{}, err
	}
	return u.Get(ctx, actor, id)
}

// company: своя компания отдаёт id, новая выписку ЕГРЮЛ, чтобы завести её в транзакции.
// Ходить в ЕГРЮЛ внутри транзакции не хотим, она держала бы соединение весь таймаут.
func (u *applicationUseCase) company(
	ctx context.Context, actor domain.Actor, inn string,
) (uuid.UUID, *domain.Company, error) {
	id, member, err := u.apps.Company(ctx, actor.ID, inn)
	switch {
	case err == nil && !member:
		return uuid.Nil, nil, fmt.Errorf("%w: вы не представитель компании %s", domain.ErrForbidden, inn)
	case !errors.Is(err, domain.ErrNotFound):
		return id, nil, err
	}
	c, err := u.companies.Lookup(ctx, inn)
	if errors.Is(err, domain.ErrNotFound) {
		return uuid.Nil, nil, fmt.Errorf("%w: компании с ИНН %s нет в ЕГРЮЛ", domain.ErrInvalid, inn)
	}
	if err != nil {
		return uuid.Nil, nil, err
	}
	return uuid.Nil, &c, nil
}

func (u *applicationUseCase) Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error) {
	app, err := u.card(ctx, actor, id)
	if err != nil || actor.Role == domain.RoleClient {
		return app, err
	}
	app.Checks, err = u.apps.Checks(ctx, id)
	return app, err
}

// card отдаёт заявку тому, кому её видно: клиенту только его компаний,
// банку всё, кроме черновиков, как и в очереди
func (u *applicationUseCase) card(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error) {
	app, err := u.apps.Card(ctx, id, owner(actor))
	if err == nil && actor.Role != domain.RoleClient && app.Status == domain.StatusDraft {
		return domain.Application{}, domain.ErrNotFound
	}
	return app, err
}

func (u *applicationUseCase) History(
	ctx context.Context, actor domain.Actor, id uuid.UUID,
) ([]domain.AuditEntry, error) {
	// у черновика журнал пустой, поэтому владение проверяем по самой заявке
	if _, err := u.card(ctx, actor, id); err != nil {
		return nil, err
	}
	events, err := u.apps.History(ctx, id)
	if err != nil || actor.Role != domain.RoleClient {
		return events, err
	}
	for i := range events {
		if events[i].ActorRole != domain.RoleClient {
			events[i].Comment = ""
		}
	}
	return events, nil
}

func (u *applicationUseCase) List(
	ctx context.Context, actor domain.Actor, f domain.ApplicationFilter,
) ([]domain.Application, int, error) {
	f.Owner = owner(actor)
	return u.apps.List(ctx, f)
}

func (u *applicationUseCase) Edit(
	ctx context.Context, actor domain.Actor, id uuid.UUID, version int, edit func(*domain.Draft),
) (domain.Application, error) {
	err := u.db.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		apps := u.apps.WithTx(tx)
		app, err := apps.Card(ctx, id, owner(actor))
		switch {
		case err != nil:
			return err
		case app.Status != domain.StatusDraft:
			return fmt.Errorf("%w: менять можно только черновик", domain.ErrIllegalTransition)
		case app.Version != version:
			return fmt.Errorf("%w: заявка уже изменилась, обновите карточку", domain.ErrConflict)
		}
		d := app.Draft
		edit(&d)
		if d.Role != app.Role {
			if err := apps.SetMember(ctx, actor.ID, app.CompanyID, d.Role); err != nil {
				return err
			}
		}
		return apps.SaveDraft(ctx, id, version, d)
	})
	if err != nil {
		return domain.Application{}, err
	}
	return u.Get(ctx, actor, id)
}

func (u *applicationUseCase) Submit(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error) {
	app, err := u.Get(ctx, actor, id)
	if err != nil {
		return domain.Application{}, err
	}
	switch {
	case app.Status == domain.StatusDraft:
		if errs := u.catalog.Ready(&app.Draft); errs != nil {
			return domain.Application{}, errs
		}
		// версия та, что проверяли: поправят анкету между чтением и переходом, будет 409
		err = u.Transition(ctx, id, domain.Move{To: domain.StatusSubmitted, Actor: actor, Version: app.Version})
		if err != nil {
			return domain.Application{}, err
		}
	case app.SubmittedAt == nil:
		return domain.Application{}, fmt.Errorf("%w: заявку не подавали, а черновик уже закрыт",
			domain.ErrIllegalTransition)
	}
	// Старт после коммита, не внутри транзакции: иначе процесс мог бы пойти по заявке,
	// переход которой откатился. Если Temporal не ответил, заявка остаётся в submitted,
	// а повторная подача дойдёт сюда же и запустит процесс с тем же id.
	err = u.StartProcess(ctx, id)
	if err != nil && !errors.Is(err, domain.ErrConflict) {
		return domain.Application{}, fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	return u.Get(ctx, actor, id)
}

func (u *applicationUseCase) Cancel(
	ctx context.Context, actor domain.Actor, id uuid.UUID, reason string,
) (domain.Application, error) {
	app, err := u.Get(ctx, actor, id)
	if err != nil {
		return domain.Application{}, err
	}
	if app.Status == domain.StatusDraft {
		err = u.Transition(ctx, id, domain.Move{To: domain.StatusCancelledByClient, Actor: actor, Comment: reason})
	} else {
		// после подачи заявку ведёт процесс, можно ли отменить, решает его валидатор
		err = fromProcess(
			u.workflows.Update(ctx, id.String(), UpdateCancel, domain.Action{Actor: actor, Comment: reason}),
		)
	}
	if err != nil {
		return domain.Application{}, err
	}
	return u.Get(ctx, actor, id)
}

func (u *applicationUseCase) ProvideInfo(ctx context.Context, actor domain.Actor, id uuid.UUID, a Answer) error {
	app, err := u.card(ctx, actor, id)
	if err != nil {
		return err
	}
	var errs domain.FieldErrors
	for _, docID := range a.Documents {
		i := slices.IndexFunc(app.Documents, func(d domain.Document) bool { return d.ID == docID })
		switch {
		case i < 0:
			errs = append(errs, domain.FieldError{Field: "document_ids", Message: "документ не из этой заявки"})
		case app.Documents[i].Kind.Recognizable() && app.Documents[i].Status != domain.DocConfirmed:
			errs = append(errs, domain.FieldError{
				Field:   "document_ids",
				Message: "сначала проверьте и подтвердите данные документа " + app.Documents[i].FileName,
			})
		}
	}
	if errs != nil {
		return errs
	}
	if a.RevenueKop != nil {
		if err := u.apps.SetRevenue(ctx, id, *a.RevenueKop); err != nil {
			return err
		}
	}
	return fromProcess(
		u.workflows.Update(ctx, id.String(), UpdateProvideInfo, domain.Action{Actor: actor, Comment: a.Comment}),
	)
}

// owner: клиент видит заявки только своих компаний, сотрудник банка все
func owner(a domain.Actor) *uuid.UUID {
	if a.Role == domain.RoleClient {
		return &a.ID
	}
	return nil
}
