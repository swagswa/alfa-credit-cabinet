package usecase

import (
	"context"
	"fmt"
	"math"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const (
	bpInOne      = 10_000
	monthsInYear = 12
)

type RecommendQuery struct {
	Goal              domain.Goal
	AmountKop         int64
	TermMonths        int
	MonthlyRevenueKop int64 // 0 если клиент не указал
}

// Ориентир по базовой ставке продукта, без надбавки за риск.
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
	// Need: цель, сумма и срок из слов клиента. Продукт по ним выбирает Recommend, не модель.
	// Сумма и срок вне рамок всех продуктов отбрасываются. Без ИИ ErrUnavailable.
	Need(ctx context.Context, text string) (domain.Need, error)
	NeedEnabled() bool
}

// NeedReader модель, которая достаёт цель, сумму и срок из текста.
type NeedReader interface {
	Need(ctx context.Context, text string) (domain.Need, error)
}

type productUseCase struct {
	catalog domain.Catalog
	reader  NeedReader
}

// NewProduct: reader nil, если стенд без профиля ai
func NewProduct(c domain.Catalog, reader NeedReader) ProductUseCase {
	return &productUseCase{catalog: c, reader: reader}
}

func (u *productUseCase) NeedEnabled() bool { return u.reader != nil }

func (u *productUseCase) Need(ctx context.Context, text string) (domain.Need, error) {
	if u.reader == nil {
		return domain.Need{}, fmt.Errorf("%w: подбор по тексту выключен", domain.ErrUnavailable)
	}
	n, err := u.reader.Need(ctx, text)
	if err != nil {
		return domain.Need{}, err
	}
	// рамки всех продуктов разом: какой продукт подойдёт, решит Recommend
	amountMin, amountMax, termMin, termMax := int64(math.MaxInt64), int64(0), math.MaxInt, 0
	for i := range u.catalog.Products {
		p := &u.catalog.Products[i]
		amountMin, amountMax = min(amountMin, p.AmountMinKop), max(amountMax, p.AmountMaxKop)
		termMin, termMax = min(termMin, p.TermMinMonths), max(termMax, p.TermMaxMonths)
	}
	n.AmountKop = n.AmountKop / u.catalog.AmountStepKop * u.catalog.AmountStepKop
	if n.AmountKop < amountMin || n.AmountKop > amountMax {
		n.AmountKop = 0
	}
	if n.TermMonths < termMin || n.TermMonths > termMax {
		n.TermMonths = 0
	}
	return n, nil
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
		v = float64(q.AmountKop) / domain.KopInRub
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
	o.MonthlyPaymentKop = payment(o.AmountKop, o.RateBP, o.TermMonths, p.Repayment)
	return o
}

// factor это платёж в месяц на копейку долга. У овердрафта тело гасится поступлениями,
// ориентир это проценты за месяц при полном лимите.
func factor(rateBP, months int, rp domain.Repayment) float64 {
	r := float64(rateBP) / bpInOne / monthsInYear
	if rp == domain.AutoDebit {
		return r
	}
	return r / (1 - math.Pow(1+r, -float64(months)))
}

func payment(amountKop int64, rateBP, months int, rp domain.Repayment) int64 {
	return int64(math.Round(float64(amountKop) * factor(rateBP, months, rp)))
}
