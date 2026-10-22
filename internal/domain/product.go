package domain

// KopInRub для перевода: суммы храним в копейках, в правилах и у клиента они в рублях
const KopInRub = 100

type ProductCode string

const (
	WorkingCapital ProductCode = "working_capital"
	Overdraft      ProductCode = "overdraft"
	Equipment      ProductCode = "equipment"
)

// ProductCodes в порядке показа клиенту.
var ProductCodes = []ProductCode{WorkingCapital, Overdraft, Equipment}

type Repayment string

const (
	Annuity   Repayment = "annuity"
	AutoDebit Repayment = "auto_debit"
)

type Goal string

const (
	GoalCashGap    Goal = "cash_gap"
	GoalPurchase   Goal = "purchase"
	GoalRenovation Goal = "renovation"
	GoalEquipment  Goal = "equipment"
	GoalOther      Goal = "other"
)

var Goals = []Goal{GoalCashGap, GoalPurchase, GoalRenovation, GoalEquipment, GoalOther}

type Product struct {
	Code                 ProductCode
	Name                 string
	Description          string
	AmountMinKop         int64
	AmountMaxKop         int64
	TermMinMonths        int
	TermMaxMonths        int
	TrancheMaxDays       int
	BaseRateBP           int
	MaxRateBP            int
	Repayment            Repayment
	MinBusinessAgeMonths int
	GuarantorOverKop     int64 // 0 значит поручитель не нужен
	Pledge               bool
	MaxCostShare         float64 // доля стоимости оборудования, 0 если не про оборудование
	Requirements         []string
	Documents            []DocumentRule
}

// DocumentRule говорит, какой документ нужен и когда. When пустой значит всегда.
type DocumentRule struct {
	Kind          string
	When          string
	Months        int
	AmountOverKop int64
}

// Input это величина из запроса, на которую смотрят условия подбора.
type Input string

const (
	InputTermMonths     Input = "term_months"
	InputAmountRub      Input = "amount_rub"
	InputAmountToIncome Input = "amount_to_monthly_revenue"
)

// Condition выполняется, если Input < Lt. Других операторов подбору пока не понадобилось.
type Condition struct {
	Input Input
	Lt    float64
}

// Pick одна строка таблицы подбора. When nil значит "всё остальное".
type Pick struct {
	When          *Condition
	Product       ProductCode
	Alternative   ProductCode
	AlternativeIf *Condition
}

type GoalRule struct {
	Goal  Goal
	Title string
	Picks []Pick
}

// Catalog это rules/products.yaml после загрузки: суммы уже в копейках, ставки в б.п.
type Catalog struct {
	Version             string
	KeyRateBP           int
	AmountStepKop       int64
	Products            []Product
	Goals               []GoalRule
	ProxyDocument       string
	OwnTurnoverMinShare float64
}

func (c Catalog) Product(code ProductCode) (Product, bool) {
	for i := range c.Products {
		if c.Products[i].Code == code {
			return c.Products[i], true
		}
	}
	return Product{}, false
}

// Ready проверяет анкету перед подачей по условиям продукта, nil значит можно подавать.
// Выручку тут не требуем: нужна ли она, видно только по нашим оборотам, это шаг проверки.
func (c Catalog) Ready(d *Draft) FieldErrors {
	p, ok := c.Product(d.Product)
	if !ok {
		return FieldErrors{{"product", "такого продукта сейчас нет"}}
	}
	var errs FieldErrors
	if d.AmountKop < p.AmountMinKop || d.AmountKop > p.AmountMaxKop || d.AmountKop%c.AmountStepKop != 0 {
		errs.add("amount_kop", "сумма от %d до %d руб., кратно %d руб.",
			p.AmountMinKop/KopInRub, p.AmountMaxKop/KopInRub, c.AmountStepKop/KopInRub)
	}
	if d.TermMonths < p.TermMinMonths || d.TermMonths > p.TermMaxMonths {
		errs.add("term_months", "срок от %d до %d мес.", p.TermMinMonths, p.TermMaxMonths)
	}
	switch {
	case p.MaxCostShare == 0:
	case d.EquipmentCostKop == 0:
		errs.add("equipment_cost_kop", "укажите стоимость оборудования")
	case float64(d.AmountKop) > p.MaxCostShare*float64(d.EquipmentCostKop):
		errs.add("amount_kop", "при такой стоимости оборудования кредит до %d руб., остальное собственный взнос",
			int64(p.MaxCostShare*float64(d.EquipmentCostKop))/KopInRub)
	}
	if !d.BKIConsent {
		errs.add("bki_consent", "без согласия на запрос в бюро кредитных историй подать нельзя")
	}
	if !d.PDConsent {
		errs.add("pd_consent", "без согласия на обработку персональных данных подать нельзя")
	}
	return errs
}

func (c Catalog) Goal(g Goal) (GoalRule, bool) {
	for _, r := range c.Goals {
		if r.Goal == g {
			return r, true
		}
	}
	return GoalRule{}, false
}
