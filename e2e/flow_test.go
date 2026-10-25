//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/api/oapi"
)

const mln = 100_000_000 // миллион рублей в копейках

// application оборотный кредит от руководителя, остальное сценарий правит сам
func application(inn string, amount int64, term int, revenue int64) oapi.NewApplication {
	return oapi.NewApplication{
		Inn:                inn,
		RepresentativeRole: oapi.RepresentativeRoleDirector,
		Product:            oapi.ProductCodeWorkingCapital,
		Purpose:            oapi.GoalPurchase,
		AmountKop:          amount,
		TermMonths:         term,
		MonthlyRevenueKop:  &revenue,
		BkiConsent:         new(true),
		PdConsent:          new(true),
	}
}

// coffee подаёт оборотный кредит кофейни и ждёт одобрения, сценарий 18 и AC01
func coffeeApproved(t *testing.T) (*user, oapi.ApplicationView) {
	c := as(t, "coffee")
	app := create(t, c, application("0000100012", 25*mln/10, 24, 12*mln/10))
	upload(t, c, app.Id, oapi.DocumentKindPassport, "0000100012_financials_a.pdf")
	submit(t, c, app.Id)
	return c, wait(t, c, app.Id, oapi.ApplicationStatusApproved)
}

func TestApproveSignIssue(t *testing.T) {
	t.Parallel()
	c, v := coffeeApproved(t)
	require.Equal(t, int64(25*mln/10), v.Offer.AmountKop)
	require.Equal(t, 24, v.Offer.TermMonths)
	require.Equal(t, 1900, v.Offer.RateBp)
	require.Contains(t, codes(v), "R_APPROVED")

	acc, err := c.AcceptOfferWithResponse(t.Context(), v.Id, oapi.VersionRequest{Version: v.Version})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, acc.StatusCode(), string(acc.Body))
	wait(t, c, v.Id, oapi.ApplicationStatusSigning)

	// AC16: три неверных кода сжигают код, заявка остаётся на подписании
	code := signingCode(t, c, v.Id)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for range 3 {
		require.Equal(t, http.StatusUnprocessableEntity, sign(t, c, v.Id, wrong))
	}
	require.Equal(t, http.StatusUnprocessableEntity, sign(t, c, v.Id, code), "сгоревший код принят")
	require.Equal(t, oapi.ApplicationStatusSigning, card(t, c, v.Id).Status)

	require.Equal(t, http.StatusOK, sign(t, c, v.Id, signingCode(t, c, v.Id)))
	wait(t, c, v.Id, oapi.ApplicationStatusClosed)

	evs := history(t, c, v.Id)
	require.True(t, hasEvent(evs, oapi.ApplicationEventKindTransition, oapi.ApplicationStatusIssued))
	require.True(t, hasEvent(evs, oapi.ApplicationEventKindSigning, ""))
}

func sign(t *testing.T, c *user, app uuid.UUID, code string) int {
	t.Helper()
	res, err := c.SignApplicationWithResponse(t.Context(), app, oapi.SignRequest{Code: code})
	require.NoError(t, err)
	return res.StatusCode()
}

func signingCode(t *testing.T, c *user, app uuid.UUID) string {
	t.Helper()
	res, err := c.RequestSigningCodeWithResponse(t.Context(), app)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	require.Equal(t, 3, res.JSON200.AttemptsLeft)
	return res.JSON200.Code
}

// AC02: оборудование логистики дают меньше и дольше, с залогом
func TestCounterOffer(t *testing.T) {
	t.Parallel()
	c := as(t, "logistics")
	body := application("000020001010", 6*mln, 36, 2*mln)
	body.RepresentativeRole, body.Product, body.Purpose = oapi.RepresentativeRoleOwnerIp, oapi.ProductCodeEquipment, oapi.GoalEquipment
	body.EquipmentCostKop = new(int64(67 * mln / 10))
	app := create(t, c, body)
	upload(t, c, app.Id, oapi.DocumentKindPassport, "000020001010_statement_a.pdf")
	upload(t, c, app.Id, oapi.DocumentKindSupplyContract, "000020001010_statement_b.pdf")
	upload(t, c, app.Id, oapi.DocumentKindFinancials, "000020001010_statement_a.pdf")
	submit(t, c, app.Id)

	v := wait(t, c, app.Id, oapi.ApplicationStatusCounterOffer)
	require.Equal(t, int64(39*mln/10), v.Offer.AmountKop)
	require.Equal(t, 48, v.Offer.TermMonths)
	require.Equal(t, 2050, v.Offer.RateBp)
	require.True(t, *v.Offer.PledgeRequired)
	require.Subset(t, codes(v), []string{"R_TERM_EXTENDED", "R_AMOUNT_DSR"})
}

// AC04: стройке отказ, банк видит балл и класс
func TestReject(t *testing.T) {
	t.Parallel()
	c := as(t, "build")
	app := create(t, c, application("0000400016", 5*mln, 12, 4*mln))
	upload(t, c, app.Id, oapi.DocumentKindPassport, "0000400016_statement_a.pdf")
	upload(t, c, app.Id, oapi.DocumentKindFinancials, "0000400016_financials_a.pdf")
	submit(t, c, app.Id)

	v := wait(t, c, app.Id, oapi.ApplicationStatusRejected)
	require.Contains(t, codes(v), "R_GRADE_E")
	bank := card(t, as(t, "manager"), app.Id)
	require.Equal(t, oapi.GradeE, *bank.Grade)
	require.NotNil(t, bank.Scoring)
}

// AC03: IT без выписки ждёт документы, после ответа уходит андеррайтеру, тот даёт
// контрпредложение, клиент принимает
func TestUnderwriter(t *testing.T) {
	t.Parallel()
	c := as(t, "it")
	app := create(t, c, application("0000300011", 8*mln, 12, 35*mln/10))
	upload(t, c, app.Id, oapi.DocumentKindPassport, "0000300011_financials_a.pdf")
	submit(t, c, app.Id)
	v := wait(t, c, app.Id, oapi.ApplicationStatusNeedInfo)
	require.Contains(t, codes(v), "SF_DOCS_MISSING")

	st := upload(t, c, app.Id, oapi.DocumentKindStatement, "0000300011_statement_a.pdf")
	confirm(t, c, st, "0000300011_statement_a.pdf")
	fin := upload(t, c, app.Id, oapi.DocumentKindFinancials, "0000300011_financials_a.pdf")
	confirm(t, c, fin, "0000300011_financials_a.pdf")
	res, err := c.ProvideInfoWithResponse(
		t.Context(),
		app.Id,
		oapi.ProvideInfoRequest{DocumentIds: &[]uuid.UUID{st.Id, fin.Id}},
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	wait(t, c, app.Id, oapi.ApplicationStatusManualReview)

	uw := as(t, "underwriter")
	rec := card(t, uw, app.Id).Recommendation
	require.NotNil(t, rec)
	decision := oapi.DecisionRequest{
		Outcome: oapi.DecisionOutcomeCounterOffer,
		Terms: &oapi.OfferTerms{
			Product:           oapi.ProductCodeWorkingCapital,
			AmountKop:         7 * mln,
			TermMonths:        12,
			RateBp:            2250,
			GuarantorRequired: new(true),
		},
		Comment: "Обороты по выписке ровные, даю чуть меньше под поручительство",
	}

	// решать может только андеррайтер, без комментария нельзя
	bad, err := as(t, "manager").DecideApplicationWithResponse(t.Context(), app.Id, decision)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, bad.StatusCode())
	noComment := decision
	noComment.Comment = "   "
	bad, err = uw.DecideApplicationWithResponse(t.Context(), app.Id, noComment)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, bad.StatusCode(), string(bad.Body))

	ok, err := uw.DecideApplicationWithResponse(t.Context(), app.Id, decision)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, ok.StatusCode(), string(ok.Body))
	again, err := uw.DecideApplicationWithResponse(t.Context(), app.Id, decision)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, again.StatusCode())

	v = wait(t, c, app.Id, oapi.ApplicationStatusCounterOffer)
	require.Equal(t, int64(7*mln), v.Offer.AmountKop)
	acc, err := c.AcceptOfferWithResponse(t.Context(), app.Id, oapi.VersionRequest{Version: v.Version})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, acc.StatusCode(), string(acc.Body))
}

// AC13 ручным вводом: приход в выписке завышен в 1,5 раза, специалист видит обе цифры
func TestDocMismatch(t *testing.T) {
	t.Parallel()
	c := as(t, "it")
	app := create(t, c, application("0000300011", 8*mln, 12, 35*mln/10))
	upload(t, c, app.Id, oapi.DocumentKindPassport, "0000300011_financials_a.pdf")
	submit(t, c, app.Id)
	wait(t, c, app.Id, oapi.ApplicationStatusNeedInfo)

	st := upload(t, c, app.Id, oapi.DocumentKindStatement, "0000300011_statement_a_trap.pdf")
	confirm(t, c, st, "0000300011_statement_a_trap.pdf")
	fin := upload(t, c, app.Id, oapi.DocumentKindFinancials, "0000300011_financials_a.pdf")
	confirm(t, c, fin, "0000300011_financials_a.pdf")
	res, err := c.ProvideInfoWithResponse(
		t.Context(),
		app.Id,
		oapi.ProvideInfoRequest{DocumentIds: &[]uuid.UUID{st.Id, fin.Id}},
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	wait(t, c, app.Id, oapi.ApplicationStatusManualReview)

	for _, r := range card(t, as(t, "manager"), app.Id).Reasons {
		if r.Code == "SF_DOC_MISMATCH" {
			require.Contains(t, r.Text, "по документу 62 999 998 ₽")
			return
		}
	}
	t.Fatal("нет SF_DOC_MISMATCH")
}

// AC05: реестр всегда 503, после ретраев ручное рассмотрение; повтор проверок
// при том же сбое возвращает заявку туда же
func TestSourceUnavailable(t *testing.T) {
	t.Parallel()
	c := as(t, "unavailable")
	app := create(t, c, application("0000500010", 1*mln, 12, 1*mln))
	upload(t, c, app.Id, oapi.DocumentKindPassport, "0000100012_financials_a.pdf")
	submit(t, c, app.Id)
	wait(t, c, app.Id, oapi.ApplicationStatusManualReview)
	m := as(t, "manager")
	require.Contains(t, reasons(history(t, m, app.Id)), oapi.ReasonCode("SF_SOURCE_UNAVAILABLE"))

	res, err := m.RetryChecksWithResponse(t.Context(), app.Id)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	require.NotEqual(t, oapi.ApplicationStatusManualReview, res.JSON200.Status)
	wait(t, c, app.Id, oapi.ApplicationStatusManualReview)
}

// AC17: отмена на запросе документов
func TestCancelOnNeedInfo(t *testing.T) {
	t.Parallel()
	c := as(t, "coffee")
	// без паспорта процесс сам попросит документы
	app := create(t, c, application("0000100012", 1*mln, 12, 12*mln/10))
	submit(t, c, app.Id)
	wait(t, c, app.Id, oapi.ApplicationStatusNeedInfo)

	res, err := c.CancelApplicationWithResponse(t.Context(), app.Id, oapi.CancelRequest{Reason: new("передумали")})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	wait(t, c, app.Id, oapi.ApplicationStatusCancelledByClient)
	evs := history(t, c, app.Id)
	last := evs[len(evs)-1]
	require.Equal(t, oapi.ActorRoleClient, last.ActorRole)
}

// AC11: клиент молчит, по таймеру expired. Срок тот же, что у стенда: по умолчанию
// 10 минут, в CI короткий
func TestNeedInfoExpires(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("ждёт срок ответа клиента")
	}
	c := as(t, "coffee")
	app := create(t, c, application("0000100012", 1*mln, 12, 12*mln/10))
	submit(t, c, app.Id)
	wait(t, c, app.Id, oapi.ApplicationStatusNeedInfo)
	waitWithin(t, c, app.Id, oapi.ApplicationStatusExpired, durationEnv("NEED_INFO_TTL", 10*time.Minute)+waitFor)
}

// AC07: повтор с тем же ключом отдаёт ту же заявку, с другим телом 409
func TestIdempotency(t *testing.T) {
	t.Parallel()
	c := as(t, "coffee")
	body := application("0000100012", 2*mln, 12, mln)
	params := &oapi.CreateApplicationParams{IdempotencyKey: uuid.NewString()}

	first, err := c.CreateApplicationWithResponse(t.Context(), params, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, first.StatusCode(), string(first.Body))
	repeat, err := c.CreateApplicationWithResponse(t.Context(), params, body)
	require.NoError(t, err)
	require.Less(t, repeat.StatusCode(), 300, string(repeat.Body))
	require.Contains(t, string(repeat.Body), first.JSON201.Id.String())

	body.AmountKop = 3 * mln
	other, err := c.CreateApplicationWithResponse(t.Context(), params, body)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, other.StatusCode())
}

// AC08-AC10, AC14: чужое, не своё действие, конфликт версии, неверный ввод
func TestAccess(t *testing.T) {
	t.Parallel()
	c := as(t, "coffee")
	app := create(t, c, application("0000100012", 2*mln, 12, mln))

	for _, login := range []string{"logistics", "manager"} {
		res, err := as(t, login).GetApplicationWithResponse(t.Context(), app.Id)
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, res.StatusCode(), login)
	}

	dec, err := c.DecideApplicationWithResponse(
		t.Context(),
		app.Id,
		oapi.DecisionRequest{Outcome: oapi.DecisionOutcomeApproved, Comment: "сам себе"},
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, dec.StatusCode())

	patch := oapi.ApplicationPatch{Version: app.Version, TermMonths: new(18)}
	p1, err := c.UpdateApplicationWithResponse(t.Context(), app.Id, patch)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, p1.StatusCode(), string(p1.Body))
	p2, err := c.UpdateApplicationWithResponse(t.Context(), app.Id, patch)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, p2.StatusCode())

	bad := map[string]func(*oapi.NewApplication){
		"инн из 11 цифр":             func(b *oapi.NewApplication) { b.Inn = "00001000121" },
		"неверная контрольная цифра": func(b *oapi.NewApplication) { b.Inn = "0000100013" },
		"нулевая сумма":              func(b *oapi.NewApplication) { b.AmountKop = 0 },
		"сумма не кратна 10 тыс":     func(b *oapi.NewApplication) { b.AmountKop = mln + 100 },
	}
	for name, edit := range bad {
		t.Run(name, func(t *testing.T) {
			body := application("0000100012", 2*mln, 12, mln)
			edit(&body)
			res := post(t, c, body)
			require.Equal(t, http.StatusBadRequest, res.StatusCode(), string(res.Body))
		})
	}

	// без согласий подать нельзя
	noConsent := application("0000100012", 2*mln, 12, mln)
	noConsent.BkiConsent, noConsent.PdConsent = nil, nil
	d := create(t, c, noConsent)
	res, err := c.SubmitApplicationWithResponse(t.Context(), d.Id)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, res.StatusCode(), string(res.Body))
}

// AC15: сотрудник видит смену статуса по SSE без перезагрузки
func TestLiveStatus(t *testing.T) {
	t.Parallel()
	c := as(t, "coffee")
	app := create(t, c, application("0000100012", 1*mln, 12, 12*mln/10))
	stream := events(t, as(t, "manager"), app.Id)
	submit(t, c, app.Id)

	timeout := time.After(waitFor)
	for {
		select {
		case ev := <-stream:
			if *ev.Status == oapi.ApplicationStatusNeedInfo {
				return
			}
		case <-timeout:
			require.Fail(t, "сотрудник не получил need_info по SSE")
		}
	}
}

// дашборд банка: только сотрудникам, период от 1 до 90 дней
func TestBankStats(t *testing.T) {
	t.Parallel()
	c := as(t, "coffee")
	app := create(t, c, application("0000100012", 1*mln, 12, 12*mln/10))
	submit(t, c, app.Id)

	res, err := as(t, "manager").BankStatsWithResponse(t.Context(), &oapi.BankStatsParams{Days: new(1)})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode(), string(res.Body))
	s := res.JSON200
	require.Equal(t, 1, s.Days)
	require.Positive(t, s.Submitted)
	total := 0
	for _, b := range s.ByStatus {
		total += b.Count
	}
	require.Equal(t, s.Submitted, total)
	require.LessOrEqual(t, s.AutoDecided, s.Decided)

	forbidden, err := c.BankStatsWithResponse(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, forbidden.StatusCode())
	bad, err := as(t, "underwriter").BankStatsWithResponse(t.Context(), &oapi.BankStatsParams{Days: new(91)})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, bad.StatusCode())
}
