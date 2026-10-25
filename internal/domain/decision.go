package domain

import "math"

// Стадии решения, как stage в таблице decisions.
const (
	StageChecks      = "checks"
	StagePolicy      = "policy"
	StageUnderwriter = "underwriter"
)

// Из этого выбирает андеррайтер.
var Verdicts = []Status{StatusApproved, StatusCounterOffer, StatusRejected}

// Данные проверок в том виде, на который смотрят правила.
// Собираются из check_results, анкеты и подтверждённых документов, суммы в копейках.
type Facts struct {
	IsIP bool

	// ЕГРЮЛ/ЕГРИП
	EGRULFound        bool
	EGRULStatus       string
	BusinessAgeMonths int
	OKVEDGroup        string // две первые цифры основного ОКВЭД
	UnreliableRecords int
	ForeignSharePct   float64

	// ФНС
	DisqualifiedDirector bool
	TaxBlockCollection   bool // блокировка счёта, код 1
	TaxBlockDeclaration  bool // блокировка счёта, код 2
	TaxArrearsKop        int64
	MassAddress          bool
	MassDirector         bool

	// БКИ
	Bankruptcy       bool // процедура сейчас или банкротство за 5 лет, по ЕГРЮЛ и БКИ
	CurrentDPDMax    int  // текущая просрочка компании или собственника, дни
	OwnerWorstDPD12m int
	WorstDPD24m      int // нижняя граница худшего бина БКИ: 0, 1, 30, 60, 90
	Inquiries90d     int
	BKIPaymentsKop   int64 // платежи по всем кредитам в месяц

	// банк и выписка
	AMLHighRisk      bool
	BankHistoryLevel int // 0 нет счёта или младше 3 мес, 1 счёт 3-11 мес, 2 счёт 12+, 3 плюс погашенный у нас кредит
	// M: меньшее из выручки ФНС / 12 и среднемесячного прихода по счёту или выписке
	MonthlyRevenueKop int64
	// T: среднемесячный кредитовый оборот по счёту или выписке, 0 если их нет
	MonthlyTurnoverKop int64
	RevenueTrend       float64 // последние 6 мес к предыдущим 6
	TurnoverCV         float64

	// анкета и документы
	RevenueMismatch    float64  // заявленная выручка против ФНС или выписки, доля
	MissingDocuments   []string // виды документов, которых не хватает
	DocRevenueMismatch float64  // из CheckRevenue
	DocRevenueKop      int64    // обе цифры сверки, для текста SF_DOC_MISMATCH
	FNSRevenueKop      int64
}

// inputs: имя input из rules/*.yaml -> значение факта, float64, bool или string.
var inputs = map[string]func(f *Facts) any{
	"egrul_status":               func(f *Facts) any { return f.EGRULStatus },
	"business_age_months":        func(f *Facts) any { return float64(f.BusinessAgeMonths) },
	"okved_group":                func(f *Facts) any { return f.OKVEDGroup },
	"unreliable_records":         func(f *Facts) any { return float64(f.UnreliableRecords) },
	"foreign_share_pct":          func(f *Facts) any { return f.ForeignSharePct },
	"disqualified_director":      func(f *Facts) any { return f.DisqualifiedDirector },
	"tax_block_collection":       func(f *Facts) any { return f.TaxBlockCollection },
	"tax_block_declaration":      func(f *Facts) any { return f.TaxBlockDeclaration },
	"tax_arrears_rub":            func(f *Facts) any { return float64(f.TaxArrearsKop) / KopInRub },
	"arrears_to_monthly_revenue": func(f *Facts) any { return ratio(f.TaxArrearsKop, f.MonthlyRevenueKop) },
	"mass_address":               func(f *Facts) any { return f.MassAddress },
	"mass_director":              func(f *Facts) any { return f.MassDirector },
	"bankruptcy":                 func(f *Facts) any { return f.Bankruptcy },
	"current_dpd_max":            func(f *Facts) any { return float64(f.CurrentDPDMax) },
	"owner_worst_dpd_12m":        func(f *Facts) any { return float64(f.OwnerWorstDPD12m) },
	"worst_dpd_24m":              func(f *Facts) any { return float64(f.WorstDPD24m) },
	"inquiries_90d":              func(f *Facts) any { return float64(f.Inquiries90d) },
	"current_dsr":                func(f *Facts) any { return ratio(f.BKIPaymentsKop, f.MonthlyRevenueKop) },
	"aml_high_risk":              func(f *Facts) any { return f.AMLHighRisk },
	"bank_history_level":         func(f *Facts) any { return float64(f.BankHistoryLevel) },
	"revenue_trend":              func(f *Facts) any { return f.RevenueTrend },
	"turnover_cv":                func(f *Facts) any { return f.TurnoverCV },
	"revenue_mismatch":           func(f *Facts) any { return f.RevenueMismatch },
	"missing_documents":          func(f *Facts) any { return float64(len(f.MissingDocuments)) },
	"doc_revenue_mismatch":       func(f *Facts) any { return f.DocRevenueMismatch },
}

// Input отдаёт значение по имени input из rules/*.yaml. false значит, что такого
// входа нет, это ловит загрузчик правил.
func (f *Facts) Input(name string) (any, bool) {
	get, ok := inputs[name]
	if !ok {
		return nil, false
	}
	return get(f), true
}

// ratio без выручки: долг есть, значит нагрузка бесконечная, долга нет, значит ноль
func ratio(a, b int64) float64 {
	switch {
	case b > 0:
		return float64(a) / float64(b)
	case a > 0:
		return math.Inf(1)
	}
	return 0
}

// Outcome: rejected, need_info или manual_review.
type StopHit struct {
	Code    string `json:"code"`
	Outcome Status `json:"outcome"`
}

// Вклад одного фактора. Reason пустой, если бин без кода.
type FactorScore struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Points int     `json:"points"`
	Max    int     `json:"max"`
	Reason string  `json:"reason,omitempty"`
}

// Риск компании, от продукта не зависит.
type Score struct {
	Points  int           `json:"points"`
	Grade   string        `json:"grade"`
	PD      string        `json:"pd"`
	Factors []FactorScore `json:"factors"`
	Version string        `json:"version"`
}

// Предложение клиенту или рекомендация андеррайтеру.
type Terms struct {
	Product    ProductCode `json:"product"`
	AmountKop  int64       `json:"amount_kop"`
	TermMonths int         `json:"term_months"`
	RateBP     int         `json:"rate_bp"`
	PaymentKop int64       `json:"payment_kop"`
	Guarantor  bool        `json:"guarantor"`
	Pledge     bool        `json:"pledge"`
}

// Text из шаблона, пустой, если клиенту
// код не показываем (мягкие стоп-факторы видит только банк).
type Reason struct {
	Code string `json:"code"`
	Text string `json:"text,omitempty"`
	Kind string `json:"kind,omitempty"`
}

// Kind причины для плана после отказа: плюс и минус скоринга, что исправить. У остальных пусто.
const (
	KindPlus  = "plus"
	KindMinus = "minus"
	KindFix   = "fix"
)

// Результат стадии. Outcome совпадает со статусом, в который идёт заявка;
// у checks без запретов он пустой, дальше скоринг. Terms у approved и counter_offer,
// Recommendation у manual_review. Решение андеррайтера несёт балл и рекомендацию
// политики, на которые он смотрел; Override, если разошёлся с рекомендацией.
type Decision struct {
	Stage          string
	Outcome        Status
	Hits           []StopHit
	Score          *Score
	Terms          *Terms
	Recommendation *Terms
	Reasons        []Reason
	// Documents каких не хватает, их запросим у клиента. В decisions не пишется
	Documents      []DocKind
	ScoringVersion string
	PolicyVersion  string
	Override       bool
}

// Codes для decisions.reason_codes.
func (d *Decision) Codes() []string {
	out := make([]string, 0, len(d.Reasons))
	for _, r := range d.Reasons {
		out = append(out, r.Code)
	}
	return out
}

// Fit как банк отнесётся к условиям, которые клиент подобрал под встречным сам.
type Fit string

const (
	FitAuto     Fit = "auto"    // одобрит без специалиста, можно принять
	FitLimited  Fit = "limited" // столько не даст, в Terms что даст
	FitManual   Fit = "manual"  // нужен специалист, это уже новая заявка
	FitRejected Fit = "rejected"
)

// fits по исходу политики: одобрение у неё бывает, только когда условия совпали с запросом
var fits = map[Status]Fit{StatusApproved: FitAuto, StatusCounterOffer: FitLimited, StatusManualReview: FitManual}

func (d *Decision) Fit() Fit {
	if f, ok := fits[d.Outcome]; ok {
		return f
	}
	return FitRejected
}

// Bin одна строка фактора: подходит, если значение < Lt; Lt nil это "всё остальное".
type Bin struct {
	Lt     *float64
	Points int
	Reason string
}

// OKVEDGroup строка отраслевого фактора; пустой OKVED это "всё остальное".
type OKVEDGroup struct {
	OKVED  []string
	Points int
	Reason string
}

// Factor из rules/scoring.yaml: заполнены либо Bins, либо Groups.
type Factor struct {
	Code   string
	Name   string
	Input  string
	Bins   []Bin
	Groups []OKVEDGroup
	Max    int
}

type GradeRule struct {
	Grade string
	Min   int
	PD    string
}

type ScoreReason struct {
	Plus bool
	Text string
}

// rules/scoring.yaml после загрузки, Grades по убыванию Min.
type ScoringRules struct {
	Version    string
	Factors    []Factor
	Grades     []GradeRule
	TopReasons int
	Reasons    map[string]ScoreReason
}

// Op из фиксированного набора, выражений в правилах нет.
type Op string

const (
	OpGt Op = "gt"
	OpLt Op = "lt"
	OpEq Op = "eq"
	OpNe Op = "ne"
)

// StopRule строка stop_factors. Value того же типа, что Facts.Input(Input).
type StopRule struct {
	Code    string
	Input   string
	Op      Op
	Value   any
	Outcome Status
}

type GradeAction string

const (
	GradeAuto   GradeAction = "auto"
	GradeManual GradeAction = "manual"
	GradeReject GradeAction = "reject"
)

// GradeTerms клетка матрицы класс x продукт. У овердрафта лимит от оборота
// (TurnoverShare), у кредитов от выручки (RevenueMult) и срок MaxTerm.
type GradeTerms struct {
	Action        GradeAction
	RevenueMult   float64
	TurnoverShare float64
	MaxTerm       int
	// поручитель при сумме больше этой, nil значит класс поручителя не требует
	GuarantorOverKop *int64
}

type ProductPolicy struct {
	AutoLimitKop int64
	Grades       map[string]GradeTerms
}

// Загруженный rules/policy.yaml: суммы в копейках, надбавки в б.п.
type PolicyRules struct {
	Version           string
	DSRMax            map[string]float64
	RatePremiumBP     map[string]int
	Products          map[ProductCode]ProductPolicy
	SwitchAgeMinShare float64
	LimitRoundKop     int64
	StopFactors       []StopRule
	// Texts шаблоны клиенту по коду, подстановки в фигурных скобках
	Texts map[string]string
	// UnderwriterTexts для причин андеррайтера, которых не было в расчёте
	UnderwriterTexts map[string]string
	// DocumentNames для подстановки {doc}: вид документа -> как его назвать клиенту
	DocumentNames map[string]string
	ReapplyMonths int // через сколько после отказа по классу E подавать снова
}
