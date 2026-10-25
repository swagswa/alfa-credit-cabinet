package usecase

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// ставки в б.п., в тексте клиенту проценты
const percent = 100

// Policy по шагам: класс E, возраст, лимиты, ручное, исход. Цифры в rules/policy.yaml.
func (u *decisionUseCase) Policy(
	app *domain.Application,
	f *domain.Facts,
	checks *domain.Decision,
	s *domain.Score,
	now time.Time,
) domain.Decision {
	d := domain.Decision{
		Stage:          domain.StagePolicy,
		Hits:           checks.Hits,
		Score:          s,
		ScoringVersion: s.Version,
		PolicyVersion:  u.policy.Version,
	}
	finish := func(outcome domain.Status, rs ...domain.Reason) domain.Decision {
		d.Outcome = outcome
		d.Reasons = slices.Concat(checks.Reasons, rs, u.scoreReasons(s))
		return d
	}
	g := u.policy.Products[app.Product].Grades[s.Grade]
	if g.Action == domain.GradeReject {
		reapply := now.AddDate(0, u.policy.ReapplyMonths, 0).Format("02.01.2006")
		return finish(domain.StatusRejected, append(
			[]domain.Reason{u.reason("R_GRADE_E", map[string]string{"reapply": reapply})}, u.fixes(f)...)...)
	}

	want := domain.Terms{Product: app.Product, AmountKop: app.AmountKop, TermMonths: app.TermMonths}
	var rs []domain.Reason
	if !u.available(want.Product, s.Grade, f) {
		lim, _ := u.limit(domain.Overdraft, s.Grade, f)
		if !u.available(domain.Overdraft, s.Grade, f) ||
			float64(lim) < u.policy.SwitchAgeMinShare*float64(want.AmountKop) {
			return finish(domain.StatusRejected, u.reason("R_AGE_PRODUCT", nil))
		}
		want.Product = domain.Overdraft
		rs = append(rs, u.reason("R_SWITCH_AGE", nil))
	}

	t, more := u.terms(want, s.Grade, f)
	if t.Product == domain.Overdraft && t.AmountKop < want.AmountKop && u.available(domain.WorkingCapital, s.Grade, f) {
		want.Product = domain.WorkingCapital
		if wc, wcMore := u.terms(want, s.Grade, f); wc.AmountKop > t.AmountKop {
			t, more = wc, append([]domain.Reason{u.reason("R_SWITCH_TO_LOAN", nil)}, wcMore...)
		}
	}
	rs = append(rs, more...)
	// другой продукт тут не ищем, кроме овердрафта на оборотный выше
	if p, _ := u.catalog.Product(t.Product); t.AmountKop < p.AmountMinKop {
		return finish(domain.StatusRejected, u.reason("R_LIMIT_TOO_LOW", nil))
	}

	var manual []domain.Reason
	if app.AmountKop > u.policy.Products[app.Product].AutoLimitKop {
		manual = append(manual, u.reason("R_MANUAL_AMOUNT", nil))
	}
	// Check с пустым исходом оставляет только мягкие стоп-факторы
	if g.Action == domain.GradeManual || len(checks.Hits) > 0 {
		manual = append(manual, u.reason("R_MANUAL_RISK", nil))
	}
	switch {
	case len(manual) > 0:
		d.Recommendation = &t
		return finish(domain.StatusManualReview, append(manual, rs...)...)
	case t.Product == app.Product && t.AmountKop == app.AmountKop && t.TermMonths == app.TermMonths:
		d.Terms = &t
		return finish(domain.StatusApproved, append([]domain.Reason{u.reason("R_APPROVED", nil)}, rs...)...)
	}
	d.Terms = &t
	return finish(domain.StatusCounterOffer, rs...)
}

func (u *decisionUseCase) available(code domain.ProductCode, grade string, f *domain.Facts) bool {
	p, _ := u.catalog.Product(code)
	return u.policy.Products[code].Grades[grade].Action != domain.GradeReject &&
		f.BusinessAgeMonths >= p.MinBusinessAgeMonths
}

// room: сколько в месяц можно платить по новому кредиту, чтобы нагрузка не вышла за предел
func (u *decisionUseCase) room(grade string, f *domain.Facts) int64 {
	return int64(u.policy.DSRMax[grade]*float64(f.MonthlyRevenueKop)) - f.BKIPaymentsKop
}

// limit: лимит по продукту и классу и код того, что его ограничило.
func (u *decisionUseCase) limit(code domain.ProductCode, grade string, f *domain.Facts) (lim int64, why string) {
	p, _ := u.catalog.Product(code)
	g := u.policy.Products[code].Grades[grade]
	why = "R_AMOUNT_MULT"
	if p.Repayment == domain.AutoDebit {
		lim, why = int64(g.TurnoverShare*float64(f.MonthlyTurnoverKop)), ""
	} else {
		lim = int64(g.RevenueMult * float64(f.MonthlyRevenueKop))
		term := min(g.MaxTerm, p.TermMaxMonths)
		if a := int64(
			float64(max(u.room(grade, f), 0)) / factor(p.BaseRateBP+u.policy.RatePremiumBP[grade], term, p.Repayment),
		); a < lim {
			lim, why = a, "R_AMOUNT_DSR"
		}
	}
	// максимум продукта на снижение не влияет: больше него заявку не подать, Ready не пустит,
	// а при смене продукта код снижения не показываем
	return min(lim, p.AmountMaxKop) / u.policy.LimitRoundKop * u.policy.LimitRoundKop, why
}

// terms считает условия по продукту из want: срок, сумму до L, поручителя, залог, ставку.
func (u *decisionUseCase) terms(want domain.Terms, grade string, f *domain.Facts) (domain.Terms, []domain.Reason) {
	p, _ := u.catalog.Product(want.Product)
	g := u.policy.Products[p.Code].Grades[grade]
	t := want
	t.RateBP = p.BaseRateBP + u.policy.RatePremiumBP[grade]
	t.TermMonths = min(max(t.TermMonths, p.TermMinMonths), p.TermMaxMonths)
	var rs []domain.Reason

	if p.Repayment == domain.Annuity {
		maxTerm := min(g.MaxTerm, p.TermMaxMonths)
		t.TermMonths = min(t.TermMonths, maxTerm)
		if t.TermMonths < maxTerm && payment(t.AmountKop, t.RateBP, t.TermMonths, p.Repayment) > u.room(grade, f) {
			t.TermMonths = maxTerm
			rs = append(rs, u.reason("R_TERM_EXTENDED", map[string]string{"term": strconv.Itoa(maxTerm)}))
		}
	}
	if lim, why := u.limit(p.Code, grade, f); t.AmountKop > lim {
		t.AmountKop = lim
		if why != "" {
			rs = append(rs, u.reason(why, map[string]string{
				"dsr":  strconv.Itoa(int(math.Round(u.policy.DSRMax[grade] * percent))),
				"mult": decimal(g.RevenueMult),
			}))
		}
	}
	t.PaymentKop = payment(t.AmountKop, t.RateBP, t.TermMonths, p.Repayment)

	var over int64 = -1
	switch {
	case g.GuarantorOverKop != nil && t.AmountKop > *g.GuarantorOverKop:
		over = *g.GuarantorOverKop
	case !f.IsIP && p.GuarantorOverKop > 0 && t.AmountKop > p.GuarantorOverKop:
		over = p.GuarantorOverKop
	}
	if t.Guarantor = over >= 0; t.Guarantor {
		rs = append(rs, u.reason("R_GUARANTOR", map[string]string{"amount": rubles(over)}))
	}
	if t.Pledge = p.Pledge; t.Pledge {
		rs = append(rs, u.reason("R_PLEDGE", nil))
	}
	if u.policy.RatePremiumBP[grade] > 0 {
		rs = append(rs, u.reason("R_RATE_PREMIUM", map[string]string{"rate": decimal(float64(t.RateBP) / percent)}))
	}
	return t, rs
}

// fixes: что клиент может исправить сам до повторной заявки. Балл и класс не раскрываем,
// только недоимку и текущую просрочку
func (u *decisionUseCase) fixes(f *domain.Facts) []domain.Reason {
	var rs []domain.Reason
	if f.TaxArrearsKop > 0 {
		rs = append(rs, u.reason("R_FIX_TAX", map[string]string{"arrears": rubles(f.TaxArrearsKop)}))
	}
	if f.CurrentDPDMax > 0 {
		rs = append(rs, u.reason("R_FIX_OVERDUE", nil))
	}
	for i := range rs {
		rs[i].Kind = domain.KindFix
	}
	return rs
}

// decimal: 20,5 и 2, без хвостовых нулей
func decimal(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

// rubles: копейки в "1 500 000"
func rubles(kop int64) string {
	const group = 3
	s := strconv.FormatInt(kop/domain.KopInRub, 10)
	for i := len(s) - group; i > 0; i -= group {
		s = s[:i] + " " + s[i:]
	}
	return s
}
