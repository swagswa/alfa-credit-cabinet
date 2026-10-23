package workflow

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/internal/worker/activity"
)

var system = domain.Actor{Role: domain.RoleSystem}

// update это действие человека: куда оно ведёт заявку и из каких статусов
// его принимают. Кому можно, решает таблица переходов.
type update struct {
	name string
	to   domain.Status
	from []domain.Status
}

var updates = []update{
	{usecase.UpdateProvideInfo, domain.StatusChecking, []domain.Status{domain.StatusNeedInfo}},
	{
		usecase.UpdateRequestInfo,
		domain.StatusNeedInfo,
		[]domain.Status{domain.StatusChecking, domain.StatusManualReview},
	},
	{usecase.UpdateRetryChecks, domain.StatusChecking, []domain.Status{domain.StatusManualReview}},
	{usecase.UpdateCancel, domain.StatusCancelledByClient, nil},
}

// process это то, что workflow помнит о заявке между шагами.
type process struct {
	id        uuid.UUID
	status    domain.Status
	codes     []string
	deadlines domain.Deadlines
	// busy: идёт переход, другое действие сейчас не принимаем
	busy bool
	// infoRequest пришёл во время проверок, исполним его после них
	infoRequest *domain.Action
}

// ApplicationWorkflow ведёт заявку от подачи до выдачи, id workflow равен id заявки.
// Стартует подача, заявка к этому моменту уже в submitted.
func ApplicationWorkflow(ctx workflow.Context, id uuid.UUID) error {
	var a *activity.ApplicationActivities

	p := &process{id: id, status: domain.StatusSubmitted}
	if err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Deadlines).Get(ctx, &p.deadlines); err != nil {
		return fmt.Errorf("deadlines: %w", err)
	}
	for _, u := range updates {
		err := workflow.SetUpdateHandlerWithOptions(ctx, u.name,
			func(ctx workflow.Context, act domain.Action) error { return p.act(ctx, u, act) },
			workflow.UpdateHandlerOptions{
				Validator: func(_ workflow.Context, act domain.Action) error { return p.validate(u, act) },
			})
		if err != nil {
			return fmt.Errorf("update handler %s: %w", u.name, err)
		}
	}

	err := p.run(ctx)
	// API ждёт ответа обработчика, без этого он получил бы ошибку вместо результата
	if waitErr := workflow.Await(ctx, func() bool { return workflow.AllHandlersFinished(ctx) }); err == nil {
		err = waitErr
	}
	if err != nil {
		return fmt.Errorf("application %s in %s: %w", id, p.status, err)
	}
	return nil
}

func (p *process) run(ctx workflow.Context) error {
	if err := p.move(ctx, domain.Move{To: domain.StatusChecking, Actor: system}); err != nil {
		return err
	}
	waits := map[domain.Status]func(workflow.Context) error{
		domain.StatusNeedInfo:     p.waitClient,
		domain.StatusManualReview: p.waitReview,
	}
	for {
		if p.status == domain.StatusChecking {
			if waiting, err := p.check(ctx); err != nil || !waiting {
				// не ждём людей: дальше стоп-факторы, скоринг и решение
				return err
			}
			continue
		}
		wait, ok := waits[p.status]
		if !ok {
			// финальный статус или шаг, которого процесс пока не ведёт
			return nil
		}
		if err := wait(ctx); err != nil {
			return err
		}
	}
}

// check опрашивает источники и решает, ждать ли людей. false: всё на месте, можно решать.
func (p *process) check(ctx workflow.Context) (bool, error) {
	statuses, err := fetchSources(ctx, p.id)
	if err != nil {
		return false, err
	}
	// стоп-факторы про сами источники; запрос сильнее ручного, как у остальных
	var codes []string
	if statuses[domain.SourceEGRUL] == domain.CheckNotFound {
		codes = append(codes, domain.CodeNotFound)
	}
	if slices.ContainsFunc(
		domain.Sources,
		func(s domain.Source) bool { return statuses[s] == domain.CheckUnavailable },
	) {
		codes = append(codes, domain.CodeSourceUnavailable)
	}

	switch {
	case p.infoRequest != nil:
		r := p.infoRequest
		p.infoRequest = nil
		m := domain.Move{
			To:      domain.StatusNeedInfo,
			Actor:   r.Actor,
			Codes:   slices.Concat(r.Codes, codes),
			Comment: r.Comment,
		}
		return true, p.move(ctx, m)
	case slices.Contains(codes, domain.CodeNotFound):
		return true, p.move(ctx, domain.Move{To: domain.StatusNeedInfo, Actor: system, Codes: codes[:1]})
	case len(codes) > 0:
		return true, p.move(ctx, domain.Move{To: domain.StatusManualReview, Actor: system, Codes: codes})
	}
	return false, nil
}

// waitClient ждёт ответа или отмены. Не дождались за срок: заявка истекает.
func (p *process) waitClient(ctx workflow.Context) error {
	left, err := workflow.AwaitWithTimeout(ctx, p.deadlines.NeedInfo, p.left(domain.StatusNeedInfo))
	if err != nil || left {
		return wrapWait(err)
	}
	// срок вышел, но клиент мог как раз отвечать: даём ему закончить
	if err := workflow.Await(ctx, func() bool { return !p.busy }); err != nil {
		return wrapWait(err)
	}
	if p.status != domain.StatusNeedInfo {
		return nil
	}
	return p.move(ctx, domain.Move{To: domain.StatusExpired, Actor: system})
}

// waitReview ждёт сотрудника. SLA только эскалирует, заявку сам по себе не двигает.
func (p *process) waitReview(ctx workflow.Context) error {
	var a *activity.ApplicationActivities

	left, err := workflow.AwaitWithTimeout(ctx, p.deadlines.Review, p.left(domain.StatusManualReview))
	if err != nil || left {
		return wrapWait(err)
	}
	if err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Escalate, p.id).Get(ctx, nil); err != nil {
		return fmt.Errorf("escalate: %w", err)
	}
	return wrapWait(workflow.Await(ctx, p.left(domain.StatusManualReview)))
}

func (p *process) left(s domain.Status) func() bool {
	return func() bool { return p.status != s && !p.busy }
}

// validate решает до записи в историю, принимать ли действие. Отказ сразу уходит в API.
func (p *process) validate(u update, a domain.Action) error {
	switch {
	case p.busy:
		return rejected("заявка как раз меняется, обновите карточку")
	case u.from != nil && !slices.Contains(u.from, p.status):
		return rejected(fmt.Sprintf("в статусе %s так нельзя", p.status))
	case u.to == domain.StatusNeedInfo && p.status == domain.StatusChecking:
		// перехода пока нет, но потом он будет от имени этого сотрудника
		if p.infoRequest != nil {
			return rejected("запрос уже отправлен")
		}
	case u.name == usecase.UpdateRetryChecks && !slices.Contains(p.codes, domain.CodeSourceUnavailable):
		return rejected("повторять проверки стоит, только если источник не ответил")
	}
	if !domain.CanTransition(p.status, u.to, a.Actor.Role) {
		return rejected(fmt.Sprintf("роль %s не может перевести заявку из %s в %s", a.Actor.Role, p.status, u.to))
	}
	return nil
}

// act исполняет принятое действие. Ошибку перехода (например, устаревшую карточку)
// отдаём в API как есть: заявка остаётся, где была, процесс ждёт дальше.
func (p *process) act(ctx workflow.Context, u update, a domain.Action) error {
	if u.to == domain.StatusNeedInfo && p.status == domain.StatusChecking {
		p.infoRequest = &a
		return nil
	}
	return p.move(ctx, domain.Move{To: u.to, Actor: a.Actor, Version: a.Version, Codes: a.Codes, Comment: a.Comment})
}

// move переводит заявку. Срок нового шага считаем здесь же, его увидят клиент и очередь.
func (p *process) move(ctx workflow.Context, m domain.Move) error {
	var a *activity.ApplicationActivities

	ttl := map[domain.Status]time.Duration{
		domain.StatusNeedInfo:     p.deadlines.NeedInfo,
		domain.StatusManualReview: p.deadlines.Review,
	}
	if d, ok := ttl[m.To]; ok {
		due := workflow.Now(ctx).Add(d)
		m.DueAt = &due
	}

	p.busy = true
	defer func() { p.busy = false }()
	// без обёртки: тип ошибки activity должен дойти до API
	if err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Transition, p.id, m).Get(ctx, nil); err != nil {
		return err //nolint:wrapcheck // см. выше
	}
	p.status, p.codes = m.To, m.Codes
	return nil
}

// wrapWait: ожидание прерывает только отмена workflow.
func wrapWait(err error) error {
	if err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	return nil
}

func rejected(msg string) error {
	return temporal.NewApplicationError(msg, usecase.IllegalTransitionErrorType)
}

// fetchSources опрашивает источники разом. Источник, который так и не ответил,
// записывается как unavailable, остальные дорабатывают.
func fetchSources(ctx workflow.Context, id uuid.UUID) (map[domain.Source]domain.CheckStatus, error) {
	var a *activity.CheckActivities

	srcCtx := WithSourceActivityOptions(ctx)
	futures := make([]workflow.Future, len(domain.Sources))
	for i, src := range domain.Sources {
		futures[i] = workflow.ExecuteActivity(srcCtx, a.FetchSource, id, src)
	}

	// ответы собираем в порядке domain.Sources, а не по готовности: так реплей детерминирован
	statuses := make(map[domain.Source]domain.CheckStatus, len(domain.Sources))
	for i, src := range domain.Sources {
		var status domain.CheckStatus
		err := futures[i].Get(ctx, &status)
		if err != nil {
			workflow.GetLogger(ctx).Warn("source unavailable", "source", src, "error", err)
			err = workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.SaveUnavailable, id, src, reason(err)).
				Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("save unavailable %s: %w", src, err)
			}
			status = domain.CheckUnavailable
		}
		statuses[src] = status
	}
	return statuses, nil
}

// reason достаёт причину из обёртки temporal: в карточку нужна она, а не id событий истории.
func reason(err error) string {
	if appErr, ok := errors.AsType[*temporal.ApplicationError](err); ok {
		return appErr.Message()
	}
	return err.Error()
}
