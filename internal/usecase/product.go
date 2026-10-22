package usecase

import (
	"fmt"
	"math"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const (
	kopInRub     = 100
	bpInOne      = 10_000
	monthsInYear = 12
)

type RecommendQuery struct {
	Goal              domain.Goal
	AmountKop         int64
	TermMonths        int
	MonthlyRevenueKop int64 // 0 если клиент не указал
}

// Offer это ориентир по базовой ставке продукта, без надбавки за риск.
type Offer struct {
	Product           domain.ProductCode
	Primary           bool
	AmountKop         int64
	TermMonths        int
	RateBP            int
	MonthlyPaymentKop int64
}

type ProductUseCase interface {
	List() []domain.Product
	// Recommend: первым рекомендованный продукт, вторым альтернатива, если она есть.
	Recommend(q RecommendQuery) ([]Offer, error)
}

type productUseCase struct {
	catalog domain.Catalog
}

func NewProduct(c domain.Catalog) ProductUseCase {
	return &productUseCase{catalog: c}
}

func (u *productUseCase) List() []domain.Product {
	return u.catalog.Products
}

func (u *productUseCase) Recommend(q RecommendQuery) ([]Offer, error) {
	rule, ok := u.catalog.Goal(q.Goal)
	if !ok || q.AmountKop <= 0 || q.TermMonths <= 0 {
		return nil, fmt.Errorf("%w: цель, сумма или срок", domain.ErrInvalid)
	}
	for _, p := range rule.Picks {
		if !q.holds(p.When) {
			continue
		}
		offers := []Offer{u.offer(p.Product, q, true)}
		if p.Alternative != "" && q.holds(p.AlternativeIf) {
			offers = append(offers, u.offer(p.Alternative, q, false))
		}
		return offers, nil
	}
	// загрузчик требует последнюю строку без условия, сюда не попадаем
	return nil, fmt.Errorf("goal %s: no pick", q.Goal)
}

// holds: условие по неизвестной величине (оборот не указан) считаем выполненным,
// так альтернатива просто показывается, а клиент решает сам
func (q RecommendQuery) holds(c *domain.Condition) bool {
	if c == nil {
		return true
	}
	var v float64
	switch c.Input {
	case domain.InputTermMonths:
		v = float64(q.TermMonths)
	case domain.InputAmountRub:
		v = float64(q.AmountKop) / kopInRub
	case domain.InputAmountToIncome:
		if q.MonthlyRevenueKop <= 0 {
			return true
		}
		v = float64(q.AmountKop) / float64(q.MonthlyRevenueKop)
	}
	return v < c.Lt
}

func (u *productUseCase) offer(code domain.ProductCode, q RecommendQuery, primary bool) Offer {
	p, _ := u.catalog.Product(code) // коды в подборе загрузчик уже сверил с продуктами
	o := Offer{
		Product:    code,
		Primary:    primary,
		AmountKop:  min(max(q.AmountKop, p.AmountMinKop), p.AmountMaxKop),
		TermMonths: min(max(q.TermMonths, p.TermMinMonths), p.TermMaxMonths),
		RateBP:     p.BaseRateBP,
	}
	r := float64(o.RateBP) / bpInOne / monthsInYear
	amount := float64(o.AmountKop)
	if p.Repayment == domain.AutoDebit {
		// у овердрафта тело гасится поступлениями, ориентир это проценты за месяц при полном лимите
		o.MonthlyPaymentKop = int64(math.Round(amount * r))
	} else {
		o.MonthlyPaymentKop = int64(math.Round(amount * r / (1 - math.Pow(1+r, -float64(o.TermMonths)))))
	}
	return o
}
