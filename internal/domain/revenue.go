package domain

import "github.com/google/uuid"

// CodeDocMismatch ставим, когда документ клиента не сходится с налоговой. Порог и исход в rules/policy.yaml.
const CodeDocMismatch = "SF_DOC_MISMATCH"

const monthsInYear = 12

type RevenueResult string

const (
	RevenueOK       RevenueResult = "ok"
	RevenueMismatch RevenueResult = "mismatch"
	// нет подтверждённого документа или выручки ФНС: сверять не с чем, решение это не держит
	RevenueNoData RevenueResult = "no_data"
)

// FNSIncome это income_last_year из ответа ФНС и год, за который она.
type FNSIncome struct {
	Year int
	Kop  int64
}

// RevenueCheck это итог сверки для карточки банка: документ, обе цифры и расхождение.
type RevenueCheck struct {
	Result   RevenueResult
	DocID    uuid.UUID
	Kind     DocKind
	Months   int   // период выписки, у отчётности 0
	DocKop   int64 // выручка за год по документу
	FNSKop   int64
	Mismatch float64 // |документ - ФНС| / ФНС, он же вход doc_revenue_mismatch
}

// CheckRevenue сверяет подтверждённые клиентом итоги документов с выручкой ФНС.
// limit это gt строки SF_DOC_MISMATCH из rules/policy.yaml. Из нескольких документов
// берём худший: если подделку потом заменили честной выпиской, смотреть всё равно надо.
func CheckRevenue(docs []Document, fns *FNSIncome, limit float64) RevenueCheck {
	res := RevenueCheck{Result: RevenueNoData}
	// новая компания могла ещё не отчитаться, делить не на что
	if fns == nil || fns.Kop <= 0 {
		return res
	}
	for i := range docs {
		annual, months, ok := docs[i].annualRevenue(fns.Year)
		if !ok {
			continue
		}
		c := RevenueCheck{
			Result:   RevenueOK,
			DocID:    docs[i].ID,
			Kind:     docs[i].Kind,
			Months:   months,
			DocKop:   annual,
			FNSKop:   fns.Kop,
			Mismatch: float64(diff(annual, fns.Kop)) / float64(fns.Kop),
		}
		if c.Mismatch > limit {
			c.Result = RevenueMismatch
		}
		if res.Result == RevenueNoData || c.Mismatch > res.Mismatch {
			res = c
		}
	}
	return res
}

// annualRevenue приводит выписку за N месяцев к году: приход / N * 12. Допущение из модели
// процесса: выручка ФНС за год равна поступлениям за 12 месяцев выписки. Короткая выписка
// у сезонного бизнеса может дать ложное расхождение, но это ручное рассмотрение, не отказ.
// Отчётность сравниваем, только если она за тот же год, что и данные ФНС.
func (d *Document) annualRevenue(year int) (kop int64, months int, ok bool) {
	if d.Status != DocConfirmed || d.Confirmed == nil {
		return 0, 0, false
	}
	switch s, f := d.Confirmed.Statement, d.Confirmed.Financials; {
	case s != nil && len(s.MonthlyIn) > 0:
		n := len(s.MonthlyIn)
		return s.TurnoverIn * monthsInYear / int64(n), n, true
	case f != nil && f.Year == year:
		return f.Revenue, 0, true
	}
	return 0, 0, false
}
