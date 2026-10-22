package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
	"github.com/swagswa/alfa-credit-cabinet/pkg/temporalsvc"
)

// Очереди и имена процессов Temporal. Живут здесь, а не в worker/workflow:
// процессы стартует usecase, а workflow сам зовёт usecase через activity.
const (
	QueueCredit = "credit"
	QueueDocs   = "docs"

	ApplicationWorkflowName = "ApplicationWorkflow"
)

// Workflows стартует процессы, на стенде это temporalsvc.Client.
type Workflows interface {
	Start(ctx context.Context, queue, id, workflow string, args ...any) error
}

type ApplicationUseCase interface {
	// Transition единственное место, где меняется статус заявки. Повтор того же
	// перехода ничего не делает, поэтому его можно звать из activity с ретраями.
	Transition(ctx context.Context, id uuid.UUID, m domain.Move) error
	// StartProcess запускает ApplicationWorkflow заявки, id процесса равен id заявки.
	// Второй запуск для той же заявки даёт ErrConflict.
	StartProcess(ctx context.Context, id uuid.UUID) error

	// Create заводит черновик по ИНН. Компании ещё нет у нас: берём её из ЕГРЮЛ,
	// и автор становится представителем. Чужую компанию так присвоить нельзя.
	// Повтор с тем же ключом отдаёт ту же заявку, с другим телом ErrKeyReused.
	Create(ctx context.Context, actor domain.Actor, key, inn string, d domain.Draft) (domain.Application, error)
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (domain.Application, error)
	List(ctx context.Context, actor domain.Actor, f domain.ApplicationFilter) ([]domain.Application, int, error)
	// Edit меняет анкету черновика той версии, которую видел клиент.
	Edit(ctx context.Context, actor domain.Actor, id uuid.UUID, version int, edit func(*domain.Draft)) (
		domain.Application, error)
	// Cancel пока только для черновика: после подачи отмена идёт через процесс заявки.
	Cancel(ctx context.Context, actor domain.Actor, id uuid.UUID, reason string) (domain.Application, error)
}

type applicationUseCase struct {
	db        *postgres.Client
	apps      repo.ApplicationRepository
	companies CompanyUseCase
	workflows Workflows
}

// NewApplication собирает usecase заявок. companies нужен только для создания черновика, воркеру можно nil.
func NewApplication(
	db *postgres.Client, apps repo.ApplicationRepository, companies CompanyUseCase, workflows Workflows,
) ApplicationUseCase {
	return &applicationUseCase{db: db, apps: apps, companies: companies, workflows: workflows}
}

func (u *applicationUseCase) StartProcess(ctx context.Context, id uuid.UUID) error {
	err := u.workflows.Start(ctx, QueueCredit, id.String(), ApplicationWorkflowName, id)
	if errors.Is(err, temporalsvc.ErrAlreadyStarted) {
		return fmt.Errorf("%w: процесс заявки уже запущен", domain.ErrConflict)
	}
	return err
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
		return apps.Move(ctx, app, m)
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
	return u.apps.Card(ctx, id, owner(actor))
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

func (u *applicationUseCase) Cancel(
	ctx context.Context, actor domain.Actor, id uuid.UUID, reason string,
) (domain.Application, error) {
	app, err := u.Get(ctx, actor, id)
	if err != nil {
		return domain.Application{}, err
	}
	if app.Status != domain.StatusDraft {
		return domain.Application{}, fmt.Errorf("%w: отменить сейчас можно только черновик",
			domain.ErrIllegalTransition)
	}
	err = u.Transition(ctx, id, domain.Move{To: domain.StatusCancelledByClient, Actor: actor, Comment: reason})
	if err != nil {
		return domain.Application{}, err
	}
	return u.Get(ctx, actor, id)
}

// owner: клиент видит заявки только своих компаний, сотрудник банка все
func owner(a domain.Actor) *uuid.UUID {
	if a.Role == domain.RoleClient {
		return &a.ID
	}
	return nil
}
