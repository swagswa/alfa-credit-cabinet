package usecase_test

import (
	"crypto/sha256"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/db"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/repo"
	"github.com/swagswa/alfa-credit-cabinet/internal/rules"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
	"github.com/swagswa/alfa-credit-cabinet/pkg/postgres"
)

func (e *appEnv) issue() usecase.IssueUseCase {
	catalog, err := rules.Products("../../rules/products.yaml")
	require.NoError(e.t, err)
	return usecase.NewIssue(&postgres.Client{Pool: e.pool}, e.uc, decisions(e.t), repo.NewApplication(db.New(e.pool)),
		repo.NewDocument(e.pool), repo.NewSigning(e.pool), e.notes, catalog, 5*time.Minute)
}

// sentCode: код из последнего уведомления клиенту, как его увидит человек в колокольчике
func (e *appEnv) sentCode(id uuid.UUID) string {
	var body string
	require.NoError(e.t, e.pool.QueryRow(e.t.Context(), `SELECT body FROM notifications
		WHERE application_id = $1 AND kind = 'signing_code' ORDER BY id DESC LIMIT 1`, id).Scan(&body))
	m := regexp.MustCompile(`Код (\d{6}) `).FindStringSubmatch(body)
	require.Len(e.t, m, 2, body)
	return m[1]
}

// offer ведёт заявку кофейни через одобрение до status, как это сделал бы процесс
func (e *appEnv) offer(status domain.Status) uuid.UUID {
	ctx := e.t.Context()
	id := e.app(domain.StatusScoring)
	terms := &domain.Terms{Product: "working_capital", AmountKop: 250_000_000, TermMonths: 24, RateBP: 1900,
		PaymentKop: 12_602_154}
	d := &domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusApproved, Terms: terms,
		Score: &domain.Score{Points: 86, Grade: "A"}, PolicyVersion: "test",
	}
	require.NoError(e.t, e.uc.Transition(ctx, id, domain.Move{To: domain.StatusApproved, Actor: system, Decision: d}))
	path := []domain.Move{{To: domain.StatusAccepted, Actor: e.client}, {To: domain.StatusSigning, Actor: system}}
	for i := range path {
		if s, _, _ := e.state(id); s == status {
			break
		}
		require.NoError(e.t, e.uc.Transition(ctx, id, path[i]))
	}
	return id
}

func (e *appEnv) code(id uuid.UUID) (hash []byte, attempts int, expires time.Time) {
	err := e.pool.QueryRow(e.t.Context(), `
		SELECT code_hash, attempts_left, expires_at FROM signing_codes WHERE application_id = $1`, id).
		Scan(&hash, &attempts, &expires)
	require.NoError(e.t, err)
	return hash, attempts, expires
}

// wrongCode: код не подошёл, ошибка на поле code, в форме подсветится поле ввода
func wrongCode(t *testing.T, err error, msg string) {
	t.Helper()
	var fields domain.FieldErrors
	require.ErrorAs(t, err, &fields)
	require.Len(t, fields, 1)
	require.Equal(t, "code", fields[0].Field)
	require.Contains(t, fields[0].Message, msg)
}

func TestRequestCode(t *testing.T) {
	e := newAppEnv(t)
	uc := e.issue()
	ctx := t.Context()

	_, err := uc.RequestCode(ctx, e.client, e.offer(domain.StatusApproved))
	require.ErrorIs(t, err, domain.ErrIllegalTransition)

	id := e.offer(domain.StatusSigning)
	_, err = uc.RequestCode(ctx, e.user("logistics"), id)
	require.ErrorIs(t, err, domain.ErrNotFound)

	c, err := uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	code := e.sentCode(id)
	require.Regexp(t, `^\d{6}$`, code)
	hash, attempts, expires := e.code(id)
	sum := sha256.Sum256([]byte(code))
	require.Equal(t, sum[:], hash)
	require.Equal(t, 3, attempts)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), expires, 10*time.Second)
	require.Equal(t, 3, c.AttemptsLeft)
	require.WithinDuration(t, expires, c.ExpiresAt, time.Millisecond)

	var notes int
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT count(*) FROM application_events
		WHERE application_id = $1 AND kind = 'signing' AND comment NOT LIKE '%'||$2||'%'`, id, code).Scan(&notes))
	require.Equal(t, 1, notes, "в журнале факт выдачи, но не сам код")
}

func TestSign(t *testing.T) {
	e := newAppEnv(t)
	uc := e.issue()
	ctx := t.Context()
	id := e.offer(domain.StatusSigning)

	wrongCode(t, uc.Sign(ctx, e.client, id, "123456"), "запросите новый") // кода ещё не просили
	require.ErrorIs(t, uc.Sign(ctx, e.client, e.offer(domain.StatusAccepted), "123456"), domain.ErrIllegalTransition)

	_, err := uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	code := e.sentCode(id)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for _, left := range []string{"осталось попыток: 2", "осталось попыток: 1", "запросите новый"} {
		wrongCode(t, uc.Sign(ctx, e.client, id, wrong), left)
	}
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый") // три ошибки сожгли код
	require.Nil(t, e.wf.args, "до процесса неверный код не доходит")

	_, err = uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	code = e.sentCode(id)
	require.ErrorIs(t, uc.Sign(ctx, e.user("logistics"), id, code), domain.ErrNotFound)
	require.NoError(t, uc.Sign(ctx, e.client, id, code))
	require.Equal(t, []any{id.String(), usecase.UpdateSign, domain.Action{Actor: e.client}}, e.wf.args)

	// двойной клик: второй раз тем же кодом уже не подписать
	e.wf.args = nil
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый")
	require.Nil(t, e.wf.args)

	_, err = uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	code = e.sentCode(id)
	// срок ставим по часам приложения: у контейнера с базой на винде часы убегают на секунды
	_, err = e.pool.Exec(ctx, "UPDATE signing_codes SET expires_at = $1", time.Now().Add(-time.Second))
	require.NoError(t, err)
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый")
}

func TestAccept(t *testing.T) {
	e := newAppEnv(t)
	id := e.offer(domain.StatusApproved)
	require.NoError(t, e.issue().Accept(t.Context(), e.client, id, 2, nil))
	require.Equal(t, []any{id.String(), usecase.UpdateAccept, domain.Action{Actor: e.client, Version: 2}}, e.wf.args)
}

// до расчёта: чужая заявка, не встречное, условия вне продукта
func TestVariantRejects(t *testing.T) {
	e := newAppEnv(t)
	uc := e.issue()
	ctx := t.Context()
	approved := e.offer(domain.StatusApproved)
	ok := usecase.Choice{AmountKop: 200_000_000, TermMonths: 24}

	_, err := uc.Variant(ctx, e.user("logistics"), approved, ok)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = uc.Variant(ctx, e.client, approved, ok)
	require.ErrorIs(t, err, domain.ErrIllegalTransition, "одобрено как просили, подбирать нечего")
	err = uc.Accept(ctx, e.client, approved, 2, &ok)
	require.ErrorIs(t, err, domain.ErrIllegalTransition)
	require.Nil(t, e.wf.args, "в процесс ничего не ушло")

	counter := e.app(domain.StatusScoring)
	d := &domain.Decision{
		Stage: domain.StagePolicy, Outcome: domain.StatusCounterOffer, Score: &domain.Score{Points: 64, Grade: "C"},
		Terms:         &domain.Terms{Product: "working_capital", AmountKop: 150_000_000, TermMonths: 24, RateBP: 2050},
		PolicyVersion: "test",
	}
	require.NoError(
		t,
		e.uc.Transition(ctx, counter, domain.Move{To: domain.StatusCounterOffer, Actor: system, Decision: d}),
	)
	for name, c := range map[string]usecase.Choice{
		"срок вне продукта":    {AmountKop: 150_000_000, TermMonths: 2},
		"больше, чем просили":  {AmountKop: 1_000_000_000, TermMonths: 24},
		"сумма не кратна шагу": {AmountKop: 150_000_100, TermMonths: 24},
	} {
		_, err = uc.Variant(ctx, e.client, counter, c)
		var fields domain.FieldErrors
		require.ErrorAs(t, err, &fields, name)
	}

	// встречное от специалиста политикой не пересчитываем: она одобрила бы больше, чем он дал
	byHand := e.app(domain.StatusManualReview)
	d.Stage, d.Score = domain.StageUnderwriter, nil
	m := domain.Move{To: domain.StatusCounterOffer, Actor: e.user("underwriter"), Comment: "даю меньше", Decision: d}
	require.NoError(t, e.uc.Transition(ctx, byHand, m))
	_, err = uc.Variant(ctx, e.client, byHand, usecase.Choice{AmountKop: 80_000_000, TermMonths: 12})
	require.ErrorIs(t, err, domain.ErrIllegalTransition)
}

func TestPrepareContractAndPayout(t *testing.T) {
	e := newAppEnv(t)
	uc := e.issue()
	ctx := t.Context()
	id := e.offer(domain.StatusAccepted)
	date := time.Date(2026, time.October, 24, 10, 0, 0, 0, time.UTC)

	require.NoError(t, uc.PrepareContract(ctx, id, date))
	require.NoError(t, uc.PrepareContract(ctx, id, date))
	var n int
	var name, ct string
	require.NoError(t, e.pool.QueryRow(ctx, `
		SELECT count(*), min(file_name), min(content_type) FROM documents
		WHERE application_id = $1 AND kind = 'loan_agreement'`, id).Scan(&n, &name, &ct))
	require.Equal(t, 1, n)
	require.Regexp(t, `^dogovor-\d+\.pdf$`, name)
	require.Equal(t, "application/pdf", ct)

	require.NoError(t, uc.Payout(ctx, id))
	require.NoError(t, uc.Payout(ctx, id))
	var amount int64
	require.NoError(t, e.pool.QueryRow(ctx, "SELECT sum(amount_kop) FROM payouts WHERE application_id = $1", id).
		Scan(&amount))
	require.Equal(t, int64(250_000_000), amount)
}
