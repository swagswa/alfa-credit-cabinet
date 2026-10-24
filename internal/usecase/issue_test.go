package usecase_test

import (
	"crypto/sha256"
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
	return usecase.NewIssue(&postgres.Client{Pool: e.pool}, e.uc, repo.NewApplication(db.New(e.pool)),
		repo.NewDocument(e.pool), repo.NewSigning(e.pool), catalog, 5*time.Minute)
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
	for _, m := range path {
		if s, _, _ := e.state(id); s == status {
			break
		}
		require.NoError(e.t, e.uc.Transition(ctx, id, m))
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

	_, _, err := uc.RequestCode(ctx, e.client, e.offer(domain.StatusApproved))
	require.ErrorIs(t, err, domain.ErrIllegalTransition)

	id := e.offer(domain.StatusSigning)
	_, _, err = uc.RequestCode(ctx, e.user("logistics"), id)
	require.ErrorIs(t, err, domain.ErrNotFound)

	code, c, err := uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
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

	code, _, err := uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for _, left := range []string{"осталось попыток: 2", "осталось попыток: 1", "запросите новый"} {
		wrongCode(t, uc.Sign(ctx, e.client, id, wrong), left)
	}
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый") // три ошибки сожгли код
	require.Nil(t, e.wf.args, "до процесса неверный код не доходит")

	code, _, err = uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	require.ErrorIs(t, uc.Sign(ctx, e.user("logistics"), id, code), domain.ErrNotFound)
	require.NoError(t, uc.Sign(ctx, e.client, id, code))
	require.Equal(t, []any{id.String(), usecase.UpdateSign, domain.Action{Actor: e.client}}, e.wf.args)

	// двойной клик: второй раз тем же кодом уже не подписать
	e.wf.args = nil
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый")
	require.Nil(t, e.wf.args)

	code, _, err = uc.RequestCode(ctx, e.client, id)
	require.NoError(t, err)
	// срок ставим по часам приложения: у контейнера с базой на винде часы убегают на секунды
	_, err = e.pool.Exec(ctx, "UPDATE signing_codes SET expires_at = $1", time.Now().Add(-time.Second))
	require.NoError(t, err)
	wrongCode(t, uc.Sign(ctx, e.client, id, code), "запросите новый")
}

func TestAccept(t *testing.T) {
	e := newAppEnv(t)
	id := e.offer(domain.StatusApproved)
	require.NoError(t, e.issue().Accept(t.Context(), e.client, id, 2))
	require.Equal(t, []any{id.String(), usecase.UpdateAccept, domain.Action{Actor: e.client, Version: 2}}, e.wf.args)
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
