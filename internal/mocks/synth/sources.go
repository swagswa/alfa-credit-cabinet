package synth

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// feature это признак от скрытого качества q: base + slope*q плюс шум noise*N(0,1), обрезанный в [lo, hi].
// Для вероятностей noise 0 и отрезок [0, 1].
type feature struct{ base, slope, noise, lo, hi float64 }

func (f feature) draw(rng *rand.Rand, q float64) float64 {
	return clamp(f.base+f.slope*q+f.noise*rng.NormFloat64(), f.lo, f.hi)
}

func (f feature) chance(rng *rand.Rand, q float64) bool { return rng.Float64() < f.draw(rng, q) }

// tier это доля компаний и диапазон q, в который они попадают
type tier struct{ share, lo, hi float64 }

type params struct {
	tiers []tier // от слабых к сильным, доли в сумме 1

	avgLog2, trend, cv feature // выручка в месяц 400 тыс * 2^avgLog2, тренд и вариация оборотов
	age                feature // возраст бизнеса, мес
	dpd                feature // худшая просрочка за 24 мес, дни
	dsr                feature // платежи по кредитам к выручке в месяц
	inquiries          feature // запросы кредитной истории за 90 дней
	arrears            feature // недоимка к выручке в месяц
	pkr                feature // рейтинг собственника в бюро
	client             feature // вероятность, что счёт у нас
	inactive           feature // вероятность ликвидации
	soft               feature // вероятность мягкого стоп-фактора (недостоверность, массовость, 115-ФЗ)
	pd                 feature // вероятность дефолта за год, только разметка для Gini, в источники не идёт
}

// mix это распределение признаков у компаний из генератора. Исходы (около 60/20/10/10)
// подгоняются только этими числами, скоринг тут не считается, его гоняет TestOutcomes в mocks.
// Доли tiers задуманы под исходы: нижние 10% к отказу, следующие 10% к ручному, 20% к встречному.
// Поменяли, перегенерили testdata/documents (make docgen). Version не поднимаем: от него зависит
// и шум у демо-компаний, а на их цифры завязаны тесты распознавания и решения.
var mix = params{
	tiers: []tier{{0.1, 0, 0.2}, {0.1, 0.2, 0.4}, {0.2, 0.4, 0.6}, {0.6, 0.6, 1}},

	avgLog2: feature{0.5, 2, 0.4, math.Inf(-1), math.Inf(1)},
	trend:   feature{0.7, 0.6, 0.1, 0.5, 1.6},
	cv:      feature{0.6, -0.4, 1.0 / 12, 0.08, 0.75},
	// моложе 13 мес не делаем: выписка у всех за 12 полных месяцев
	age:       feature{14, 100, 20, 13, 240},
	dpd:       feature{45, -120, 20, 0, 120},
	dsr:       feature{0.2, -0.13, 0.06, 0, 0.6},
	inquiries: feature{7, -6, 1.5, 0, 15},
	arrears:   feature{0.08, -0.25, 0.03, 0, 0.3},
	pkr:       feature{540, 320, 45, 300, 999},
	client:    feature{0.3, 0.5, 0, 0, 1},
	inactive:  feature{0.06, -0.06, 0, 0, 1},
	soft:      feature{0.12, -0.1, 0, 0, 1},
	pd:        feature{0.45, -0.6, 0, 0.01, 1},
}

// quality переводит равномерное u в q по долям tiers
func (p params) quality(u float64) float64 {
	for _, t := range p.tiers {
		if u < t.share {
			return t.lo + (t.hi-t.lo)*u/t.share
		}
		u -= t.share
	}
	return 1
}

type okved struct{ code, name string }

var okveds = []okved{
	{"56.10", "Деятельность ресторанов и услуги по доставке продуктов питания"},
	{"47.11", "Торговля розничная преимущественно пищевыми продуктами в неспециализированных магазинах"},
	{"46.90", "Торговля оптовая неспециализированная"},
	{"49.41", "Деятельность автомобильного грузового транспорта"},
	{"62.01", "Разработка компьютерного программного обеспечения"},
	{"43.21", "Производство электромонтажных работ"},
	{"41.20", "Строительство жилых и нежилых зданий"},
	{"10.71", "Производство хлеба и мучных кондитерских изделий"},
	{"68.20", "Аренда и управление собственным или арендованным недвижимым имуществом"},
	{"86.23", "Стоматологическая практика"},
	{"85.41", "Образование дополнительное детей и взрослых"},
	{"45.20", "Техническое обслуживание и ремонт автотранспортных средств"},
}

// город по коду региона из ИНН; регионов без записи хватает, им достаётся райцентр без области
var cities = map[string]string{
	"02": "Респ. Башкортостан, г. Уфа",
	"16": "Респ. Татарстан, г. Казань",
	"23": "Краснодарский край, г. Краснодар",
	"24": "Красноярский край, г. Красноярск",
	"25": "Приморский край, г. Владивосток",
	"34": "Волгоградская обл., г. Волгоград",
	"36": "Воронежская обл., г. Воронеж",
	"38": "Иркутская обл., г. Иркутск",
	"50": "Московская обл., г. Химки",
	"52": "Нижегородская обл., г. Нижний Новгород",
	"54": "Новосибирская обл., г. Новосибирск",
	"55": "Омская обл., г. Омск",
	"59": "Пермский край, г. Пермь",
	"61": "Ростовская обл., г. Ростов-на-Дону",
	"63": "Самарская обл., г. Самара",
	"66": "Свердловская обл., г. Екатеринбург",
	"72": "Тюменская обл., г. Тюмень",
	"74": "Челябинская обл., г. Челябинск",
	"77": "г. Москва",
	"78": "г. Санкт-Петербург",
}

// мягкие стоп-факторы, у компании не больше одного, 0 значит нет
const (
	softUnreliable = iota + 1
	softMassAddress
	softMassDirector
	softForeign
	softAML
	softCount
)

// gen держит то, что нужно нескольким источникам сразу
type gen struct {
	rng       *rand.Rand
	c         Company
	month     float64 // выручка в месяц, руб
	now, reg  time.Time
	age, dpd  int
	headcount int
	soft      int
}

// Sources собирает ответы ЕГРЮЛ/ЕГРИП, ФНС, БКИ и данных банка в том же виде, что фикстуры
// моков: деньги в рублях, выручку и обороты мок досчитывает из New. Счёта в банке нет, нет и internal.
func Sources(inn string) (map[string]any, error) {
	c, err := New(inn)
	if err != nil {
		return nil, err
	}
	g := gen{rng: RNG(inn + Version + "/sources"), c: c, month: float64(c.Income()/domain.KopInRub) / 12}
	g.now = c.Months[len(c.Months)-1].Start.AddDate(0, 1, 0)
	g.age = int(g.draw(mix.age))
	g.reg = g.now.AddDate(0, -g.age, -g.rng.IntN(28))
	if g.chance(mix.soft) {
		g.soft = 1 + g.rng.IntN(softCount-1)
	}
	g.dpd = int(g.draw(mix.dpd))
	g.headcount = int(clamp(g.month*12/4e6*(0.6+0.8*g.rng.Float64()), 1, 250))

	out := map[string]map[string]any{"egrul": g.egrul(), "fns": g.fns(), "bki": g.bki(), "internal": g.bank()}
	res := map[string]any{}
	for src, body := range out {
		if body != nil {
			body["generator_version"] = Version
			res[src] = body
		}
	}
	return res, nil
}

func (g *gen) draw(f feature) float64 { return f.draw(g.rng, g.c.q) }
func (g *gen) chance(f feature) bool  { return f.chance(g.rng, g.c.q) }

func (g *gen) egrul() map[string]any {
	c, region := g.c, g.c.INN[:2]
	city, ok := cities[region]
	if !ok {
		city = "г. " + pick(g.rng, []string{"Заречный", "Советск", "Октябрьский", "Первомайск", "Каменск"})
	}
	street := pick(g.rng, []string{"ул. Ленина", "ул. Садовая", "ул. Мира", "пр. Строителей", "ул. Заводская"})
	act := pick(g.rng, okveds)
	status := "active"
	if g.chance(mix.inactive) {
		status = "liquidating"
		if c.IsIP() {
			status = "ip_terminated" // ИП не ликвидируют, он прекращает деятельность
		}
	}
	r := map[string]any{
		"inn":  c.INN, //nolint:goconst // ключи ответа как в фикстурах, константа только мешает читать
		"ogrn": ogrn(g.rng, c.IsIP(), g.reg, region), "kind": domain.KindUL,
		"full_name":  "Общество с ограниченной ответственностью " + strings.TrimPrefix(c.Name, "ООО "),
		"short_name": c.Name, "status": status, "registered_at": date(g.reg),
		"address": map[string]any{
			"region_code": region, "text": city + ", " + street + ", д. " + strconv.Itoa(1+g.rng.IntN(120)),
		},
		"okved_main":  map[string]any{"code": act.code, "name": act.name},
		"okved_extra": []any{}, "unreliable": []any{},
	}
	if g.soft == softUnreliable {
		r["unreliable"] = []any{map[string]any{"field": "address", "since": date(g.reg.AddDate(1, 0, 0))}}
	}
	if c.IsIP() {
		person := strings.TrimPrefix(c.Name, "ИП ")
		r["kind"], r["full_name"] = domain.KindIP, "Индивидуальный предприниматель "+person
		r["short_name"] = "ИП " + initials(person)
		return r
	}
	// руководитель он же единственный учредитель, как у большинства микробизнеса
	person, personINN := fio(g.rng), domain.CompleteINN(region+digits(g.rng, 8))
	r["kpp"] = region + strconv.Itoa(10+g.rng.IntN(40)) + "01001"
	r["director"] = map[string]any{
		"fio":      person,
		"inn":      personINN,
		"position": "Генеральный директор",
		"since":    date(g.reg),
	}
	r["founders"] = []any{
		map[string]any{"name": person, "inn": personINN, "share_pct": 100, "is_foreign": g.soft == softForeign},
	}
	r["capital_rub"] = 10000
	return r
}

func (g *gen) fns() map[string]any {
	year := g.month * 12
	msp, regime := "micro", "USN"
	switch {
	case year >= 800e6:
		msp, regime = "medium", "OSN"
	case year >= 450e6:
		msp, regime = "small", "OSN"
	case year >= 120e6:
		msp = "small"
	case g.c.IsIP() && year < 60e6 && g.rng.IntN(5) == 0:
		regime = "PSN"
	}
	// недоимку округляем до тысяч, мелочь уходит в ноль
	tax := round(g.month*g.draw(mix.arrears), 1000)
	offenses := 0
	if tax > 0 {
		offenses = g.rng.IntN(3)
	}
	return map[string]any{
		"msp_category":         msp,
		"tax_regime":           regime,
		"avg_headcount":        g.headcount,
		"taxes_paid_last_year": round(year*(0.02+0.04*g.rng.Float64()), 1000),
		"arrears": map[string]any{
			"tax":     tax,
			"penalty": math.Round(tax * 0.06),
			"fine":    0,
			"as_of":   date(g.now),
		},
		"mass_address":          g.soft == softMassAddress,
		"mass_director":         g.soft == softMassDirector,
		"disqualified_director": false,
		"account_blocks":        []any{},
		"tax_offenses_count":    offenses,
	}
}

func (g *gen) bki() map[string]any {
	payments := g.month * g.draw(mix.dsr)
	open := 0
	if payments > g.month/100 {
		open = 1 + g.rng.IntN(3)
	}
	closed := g.rng.IntN(2)
	if open+closed == 0 && g.dpd > 0 {
		closed = 1
	}

	loans := make([]any, 0, open+closed)
	var debt, monthly, overdue float64
	for i := range open + closed {
		var pay float64
		if i < open {
			pay = round(payments/float64(open), 1000)
		}
		l, balance := g.loan(pay)
		debt, monthly = debt+balance, monthly+pay
		// худшая просрочка на первом счёте; 90+ значит, что она идёт и сейчас
		if i == 0 && g.dpd > 0 {
			l["worst_dpd_24m"] = bucket(g.dpd)
		}
		if i == 0 && g.dpd >= 90 && pay > 0 {
			overdue = pay * 3
			l["current_dpd"], l["overdue_amount"] = g.dpd, overdue
		}
		loans = append(loans, l)
	}
	overdue30 := 0
	if g.dpd >= 30 {
		overdue30 = 1
	}

	inq90 := int(g.draw(mix.inquiries))
	inq365 := inq90 + g.rng.IntN(6)
	ownerDPD := 0
	if g.dpd > 0 {
		ownerDPD = g.rng.IntN(g.dpd + 1)
	}
	return map[string]any{
		"summary": map[string]any{
			"accounts_total": open + closed, "accounts_open": open, "accounts_overdue_30": overdue30,
			"debt_total": debt, "overdue_total": overdue, "monthly_payments_total": monthly,
			"inquiries_30d": g.rng.IntN(inq90 + 1), "inquiries_90d": inq90, "inquiries_365d": inq365,
			"applications_refused_365d": g.rng.IntN(1 + inq365/4), "bankruptcy_5y": false,
		},
		"loans": loans,
		"owner": map[string]any{"pkr": int(g.draw(mix.pkr)), "worst_dpd_12m": ownerDPD, "current_dpd": 0},
	}
}

// loan даёт счёт в бюро и остаток долга; без платежа счёт уже закрыт
func (g *gen) loan(pay float64) (l map[string]any, balance float64) {
	l = map[string]any{
		"lender_type":     "bank",
		"kind":            pick(g.rng, []string{"loan", "loan", "credit_line", "overdraft"}),
		"role":            "borrower",
		"monthly_payment": pay,
		"overdue_amount":  0,
		"current_dpd":     0,
		"worst_dpd_24m":   "0",
	}
	if pay == 0 {
		opened := g.now.AddDate(0, -25-g.rng.IntN(12), 0)
		l["opened_at"], l["closed_at"], l["status"] = date(opened), date(opened.AddDate(1, 0, 0)), "closed"
		l["amount"], l["balance"] = round(g.month*(0.5+1.5*g.rng.Float64()), 10000), 0
		return l, 0
	}
	term := 24 + 12*g.rng.IntN(3)
	passed := 1 + g.rng.IntN(term-1)
	amount := round(pay*float64(term)*0.85, 10000)
	balance = round(amount*float64(term-passed)/float64(term), 1000)
	l["opened_at"], l["closed_at"], l["status"] = date(g.now.AddDate(0, -passed, 0)), nil, "active"
	l["amount"], l["balance"] = amount, balance
	return l, balance
}

// bank отдаёт данные банка без оборотов или nil, если счёта у нас нет
func (g *gen) bank() map[string]any {
	if !g.chance(mix.client) {
		return nil
	}
	held := 1 + g.rng.IntN(g.age-1)
	products := []any{"account"}
	if g.rng.IntN(2) == 0 {
		products = append(products, "acquiring")
	}
	if g.headcount > 10 {
		products = append(products, "salary")
	}
	past := []any{}
	if held > 18 && g.rng.Float64() < 0.4 {
		past = append(past, map[string]any{
			"amount":    round(g.month*(1+2*g.rng.Float64()), 10000),
			"closed_at": date(g.now.AddDate(0, -1-g.rng.IntN(12), 0)), "worst_dpd": g.dpd / 3,
		})
	}
	return map[string]any{
		"is_client": true, "account_opened_at": date(g.now.AddDate(0, -held, 0)), "products": products,
		"past_loans": past, "aml_high_risk": g.soft == softAML,
	}
}

// ogrn: признак записи (1 юрлицо, 3 ИП), год, регион, номер и контрольная цифра,
// остаток от деления на 11 у ОГРН и на 13 у ОГРНИП
func ogrn(rng *rand.Rand, ip bool, reg time.Time, region string) string {
	s, n, mod := "1", 7, int64(11)
	if ip {
		s, n, mod = "3", 9, 13
	}
	s += reg.Format("06") + region + digits(rng, n)
	v, _ := strconv.ParseInt(s, 10, 64)
	return s + strconv.FormatInt(v%mod%10, 10)
}

func bucket(dpd int) string {
	switch {
	case dpd < 30:
		return "1-29"
	case dpd < 60:
		return "30-59"
	case dpd < 90:
		return "60-89"
	}
	return "90+"
}

func digits(rng *rand.Rand, n int) string {
	return fmt.Sprintf("%0*d", n, rng.Int64N(int64(math.Pow10(n))))
}

// initials: «Карпов Андрей Ильич» в «Карпов А. И.»
func initials(fio string) string {
	p := strings.Fields(fio)
	for i := 1; i < len(p); i++ {
		p[i] = string([]rune(p[i])[:1]) + "."
	}
	return strings.Join(p, " ")
}

func round(x, step float64) float64 { return math.Round(x/step) * step }

func date(t time.Time) string { return t.Format(time.DateOnly) }
