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
// его принимают. Кому можно, решает таблица переходов. Пустой to: исход в самом действии.
type update struct {
	name string
	to   domain.Status
	from []domain.Status
}

func (u update) target(a domain.Action) domain.Status {
	if u.to == "" {
		return a.Outcome
	}
	return u.to
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
	{usecase.UpdateDecide, "", []domain.Status{domain.StatusManualReview}},
	{
		usecase.UpdateAccept,
		domain.StatusAccepted,
		[]domain.Status{domain.StatusApproved, domain.StatusCounterOffer},
	},
	{usecase.UpdateSign, domain.StatusIssued, []domain.Status{domain.StatusSigning}},
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
	// decision последнего расчёта, на него смотрит андеррайтер
	decision *domain.Decision
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
				Validator: func(_ workflow.Context, act domain.Action) error {
					// переход уже в базе и клиент его видит, а процесс ещё ждёт ответа activity:
					// решит обработчик, когда дождётся
					if p.busy {
						return nil
					}
					return p.validate(u, act)
				},
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
	// шаги, где процесс ждёт людей или сроков; сам он двигает заявку только из accepted и issued
	waits := map[domain.Status]func(workflow.Context) error{
		domain.StatusNeedInfo:     p.waitClient(domain.StatusNeedInfo, p.deadlines.NeedInfo),
		domain.StatusManualReview: p.waitReview,
		domain.StatusApproved:     p.waitClient(domain.StatusApproved, p.deadlines.Offer),
		domain.StatusCounterOffer: p.waitClient(domain.StatusCounterOffer, p.deadlines.Offer),
		domain.StatusAccepted:     p.contract,
		domain.StatusSigning:      p.waitClient(domain.StatusSigning, p.deadlines.Signing),
		domain.StatusIssued:       p.close,
	}
	for {
		if p.status == domain.StatusChecking {
			waiting, err := p.check(ctx)
			if err == nil && !waiting {
				err = p.decide(ctx)
			}
			if err != nil {
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
			To:        domain.StatusNeedInfo,
			Actor:     r.Actor,
			Codes:     slices.Concat(r.Codes, codes),
			Comment:   r.Comment,
			Documents: r.Documents,
		}
		return true, p.move(ctx, m)
	case slices.Contains(codes, domain.CodeNotFound):
		return true, p.move(ctx, domain.Move{To: domain.StatusNeedInfo, Actor: system, Codes: codes[:1]})
	case len(codes) > 0:
		return true, p.move(ctx, domain.Move{To: domain.StatusManualReview, Actor: system, Codes: codes})
	}
	return false, nil
}

// decide считает решение и ведёт заявку в исход. Стоп-факторы останавливают
// ещё на checking, иначе заявка проходит через scoring.
func (p *process) decide(ctx workflow.Context) error {
	var a *activity.ApplicationActivities

	var d domain.Decision
	// пока считаем, запрос сотрудника не принимаем: исполнить его было бы некогда
	p.busy = true
	err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Decide, p.id).Get(ctx, &d)
	p.busy = false
	if appErr, ok := errors.AsType[*temporal.ApplicationError](err); ok && appErr.Type() == usecase.InvalidErrorType {
		// ответ источника не разобрать: решит специалист, после починки источника повторит проверки
		return p.move(ctx, domain.Move{
			To:      domain.StatusManualReview,
			Actor:   system,
			Codes:   []string{domain.CodeSourceUnavailable},
			Comment: reason(err),
		})
	}
	if err != nil {
		return fmt.Errorf("decide: %w", err)
	}
	if d.Stage == domain.StagePolicy {
		if err := p.move(ctx, domain.Move{To: domain.StatusScoring, Actor: system}); err != nil {
			return err
		}
	}
	return p.move(ctx, domain.Move{To: d.Outcome, Actor: system, Codes: d.Codes(), Decision: &d})
}

// waitClient ждёт клиента в статусе s: ответа, принятия условий, подписи или отмены.
// Не дождались за срок: заявка истекает.
func (p *process) waitClient(s domain.Status, ttl time.Duration) func(workflow.Context) error {
	return func(ctx workflow.Context) error {
		left, err := workflow.AwaitWithTimeout(ctx, ttl, p.left(s))
		if err != nil || left {
			return wrapWait(err)
		}
		// срок вышел, но клиент мог как раз действовать: даём ему закончить
		if err := workflow.Await(ctx, func() bool { return !p.busy }); err != nil {
			return wrapWait(err)
		}
		if p.status != s {
			return nil
		}
		return p.move(ctx, domain.Move{To: domain.StatusExpired, Actor: system})
	}
}

// contract готовит договор на дату принятия условий и отдаёт заявку на подпись.
func (p *process) contract(ctx workflow.Context) error {
	var a *activity.ApplicationActivities

	err := workflow.ExecuteActivity(WithContractActivityOptions(ctx), a.PrepareContract, p.id, workflow.Now(ctx)).
		Get(ctx, nil)
	if err != nil {
		return fmt.Errorf("contract: %w", err)
	}
	return p.move(ctx, domain.Move{To: domain.StatusSigning, Actor: system})
}

// close: деньги выданы, после постобработки заявка закрывается.
func (p *process) close(ctx workflow.Context) error {
	if err := workflow.Sleep(ctx, p.deadlines.Close); err != nil {
		return wrapWait(err)
	}
	return p.move(ctx, domain.Move{To: domain.StatusClosed, Actor: system})
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

// validate решает, принимать ли действие. Из валидатора отказ уходит в API без записи
// в историю, обработчик повторяет проверку после перехода, который шёл в момент запроса.
func (p *process) validate(u update, a domain.Action) error {
	switch {
	case u.from != nil && !slices.Contains(u.from, p.status):
		return rejected(fmt.Sprintf("в статусе %s так нельзя", p.status))
	case u.to == domain.StatusNeedInfo && p.status == domain.StatusChecking:
		// перехода пока нет, но потом он будет от имени этого сотрудника
		if p.infoRequest != nil {
			return rejected("запрос уже отправлен")
		}
	case u.name == usecase.UpdateRetryChecks && !slices.Contains(p.codes, domain.CodeSourceUnavailable):
		return rejected("повторять проверки стоит, только если источник не ответил")
	case u.name == usecase.UpdateDecide && (!slices.Contains(domain.Verdicts, a.Outcome) || a.Comment == "" ||
		a.Outcome != domain.StatusRejected && a.Terms == nil):
		// API это уже проверил, но без условий или комментария база решение не примет
		return rejected("решение это одобрение, контрпредложение с условиями или отказ, всегда с комментарием")
	}
	if to := u.target(a); !domain.CanTransition(p.status, to, a.Actor.Role) {
		return rejected(fmt.Sprintf("роль %s не может перевести заявку из %s в %s", a.Actor.Role, p.status, to))
	}
	return nil
}

// act исполняет принятое действие. Ошибку перехода (например, устаревшую карточку)
// отдаём в API как есть: заявка остаётся, где была, процесс ждёт дальше.
func (p *process) act(ctx workflow.Context, u update, a domain.Action) error {
	if err := workflow.Await(ctx, func() bool { return !p.busy }); err != nil {
		return wrapWait(err)
	}
	if err := p.validate(u, a); err != nil {
		return err
	}
	if u.to == domain.StatusNeedInfo && p.status == domain.StatusChecking {
		p.infoRequest = &a
		return nil
	}
	m := domain.Move{
		To: u.target(a), Actor: a.Actor, Version: a.Version, Codes: a.Codes, Comment: a.Comment, Documents: a.Documents,
		RevenueKop: a.RevenueKop,
	}
	switch u.name {
	case usecase.UpdateDecide:
		m.Decision = p.verdict(a)
	case usecase.UpdateSign:
		// код уже сверил usecase, тут только выдача; срок подписи на ней не сработает
		var acts *activity.ApplicationActivities
		p.busy = true
		err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), acts.Payout, p.id).Get(ctx, nil)
		p.busy = false
		if err != nil {
			return err
		}
	}
	return p.move(ctx, m)
}

// verdict это решение андеррайтера поверх расчёта, который он видел. Тексты причин
// берём из расчёта: они там с числами заявки. Причину, которой в расчёте не было,
// клиент не увидит, банк увидит код.
func (p *process) verdict(a domain.Action) *domain.Decision {
	var d domain.Decision
	if p.decision != nil {
		d = *p.decision
	}
	rec := d.Recommendation
	d.Override = rec != nil && (a.Terms == nil || *a.Terms != *rec)
	reasons := make([]domain.Reason, 0, len(a.Codes))
	for _, c := range a.Codes {
		i := slices.IndexFunc(d.Reasons, func(r domain.Reason) bool { return r.Code == c })
		if i < 0 {
			reasons = append(reasons, domain.Reason{Code: c})
			continue
		}
		reasons = append(reasons, d.Reasons[i])
	}
	d.Stage, d.Outcome, d.Terms, d.Reasons = domain.StageUnderwriter, a.Outcome, a.Terms, reasons
	return &d
}

// move переводит заявку. Срок нового шага считаем здесь же, его увидят клиент и очередь.
func (p *process) move(ctx workflow.Context, m domain.Move) error {
	var a *activity.ApplicationActivities

	ttl := map[domain.Status]time.Duration{
		domain.StatusNeedInfo:     p.deadlines.NeedInfo,
		domain.StatusManualReview: p.deadlines.Review,
		domain.StatusApproved:     p.deadlines.Offer,
		domain.StatusCounterOffer: p.deadlines.Offer,
		domain.StatusSigning:      p.deadlines.Signing,
	}
	if d, ok := ttl[m.To]; ok {
		due := workflow.Now(ctx).Add(d)
		m.DueAt = &due
	}

	p.busy = true
	defer func() { p.busy = false }()
	// без обёртки: тип ошибки activity должен дойти до API
	if err := workflow.ExecuteActivity(WithDBActivityOptions(ctx), a.Transition, p.id, m).Get(ctx, nil); err != nil {
		return err
	}
	p.status, p.codes = m.To, m.Codes
	if m.Decision != nil || m.To == domain.StatusChecking {
		// новые проверки: старый расчёт андеррайтеру уже не основание
		p.decision = m.Decision
	}
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
