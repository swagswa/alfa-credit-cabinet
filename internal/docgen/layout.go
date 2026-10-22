package docgen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

// Страница описана один раз, в миллиметрах A4, а рисуют её два движка:
// fpdf для текстовых PDF и x/image для «скана».
type text struct {
	x, y  float64 // y по базовой линии
	size  float64 // кегль, пт
	bold  bool
	right bool // x это правый край
	s     string
}

type page struct {
	texts []text
	rules []float64 // горизонтальные линии во всю ширину текста, по y
}

func (p *page) add(x, y, size float64, s string) {
	p.texts = append(p.texts, text{x: x, y: y, size: size, s: s})
}

func (p *page) bold(x, y, size float64, s string) {
	p.texts = append(p.texts, text{x: x, y: y, size: size, bold: true, s: s})
}

func (p *page) num(x, y, size float64, s string) {
	p.texts = append(p.texts, text{x: x, y: y, size: size, right: true, s: s})
}

func (p *page) rule(y float64) { p.rules = append(p.rules, y) }

// поля страницы, мм
const left, right = 20, 190

const footer = "Синтетический документ для демонстрации прототипа, не является документом реального банка или организации."

// statementA: «банк А», шапка, обороты по месяцам с остатком, итоги внизу
func statementA(c synth.Company) page {
	var p page
	from, to := period(c)
	p.bold(20, 22, 15, "АО «Банк А»")
	p.add(20, 28, 8.5, "БИК 044500001, к/с 30101810400000000001, г. Москва")
	p.bold(20, 44, 13, "Выписка по расчётному счёту")
	p.add(20, 51, 10, "за период с "+from+" по "+to)
	p.add(20, 62, 10, "Клиент: "+c.Name)
	p.add(20, 68, 10, "ИНН: "+c.INN)
	p.add(20, 74, 10, "Счёт: "+account(c))

	p.bold(20, 88, 9.5, "Месяц")
	p.bold(78, 88, 9.5, "Обороты по дебету")
	p.bold(118, 88, 9.5, "Обороты по кредиту")
	p.bold(158, 88, 9.5, "Остаток на конец")
	p.rule(91)
	y, bal := 97.0, c.Opening
	for _, m := range c.Months {
		bal += m.In - m.Out
		p.add(20, y, 9.5, m.Start.Format("01.2006"))
		p.num(110, y, 9.5, moneyA(m.Out))
		p.num(150, y, 9.5, moneyA(m.In))
		p.num(190, y, 9.5, moneyA(bal))
		y += 6.5
	}
	p.rule(y - 3)

	in, out := c.Turnover()
	y += 8
	for _, r := range []struct {
		label string
		v     int64
	}{
		{"Входящий остаток", c.Opening},
		{"Обороты по дебету", out},
		{"Обороты по кредиту", in},
		{"Исходящий остаток", c.Closing()},
	} {
		p.add(110, y, 10, r.label)
		p.num(190, y, 10, moneyA(r.v))
		y += 6.5
	}
	p.add(20, 285, 7.5, footer)
	return p
}

// statementB: «банк Б», итоги сверху, месяцы словами, другие подписи и формат чисел
func statementB(c synth.Company) page {
	var p page
	from, to := period(c)
	p.texts = append(p.texts, text{x: 190, y: 20, size: 14, bold: true, right: true, s: "ООО «Банк Б»"})
	p.num(190, 26, 8.5, "Генеральная лицензия № 0001")
	p.bold(20, 42, 12, "ВЫПИСКА ОПЕРАЦИЙ ПО СЧЁТУ № "+account(c))
	p.add(20, 50, 10, "Владелец счёта: "+c.Name+", ИНН "+c.INN)
	p.add(20, 56, 10, "Период: "+from+" - "+to)

	in, out := c.Turnover()
	y := 70.0
	for _, r := range []struct {
		label string
		v     int64
	}{
		{"Остаток на начало периода:", c.Opening},
		{"Поступления:", in},
		{"Списания:", out},
		{"Остаток на конец периода:", c.Closing()},
	} {
		p.add(20, y, 10, r.label)
		p.num(120, y, 10, moneyB(r.v)+" руб.")
		y += 6
	}

	y += 10
	p.bold(20, y, 9.5, "Месяц")
	p.bold(95, y, 9.5, "Поступления, руб.")
	p.bold(145, y, 9.5, "Списания, руб.")
	p.rule(y + 2)
	y += 8
	for _, m := range c.Months {
		p.add(20, y, 9.5, monthNames[m.Start.Month()-1]+" "+strconv.Itoa(m.Start.Year()))
		p.num(130, y, 9.5, moneyB(m.In))
		p.num(180, y, 9.5, moneyB(m.Out))
		y += 6
	}
	p.add(20, 285, 7.5, footer)
	return p
}

// financialsA: упрощённые баланс и отчёт о финансовых результатах на одной странице,
// расходы в скобках, как в форме
func financialsA(c synth.Company) page {
	var p page
	total, long, short, net, revenue := c.BalanceTotal, c.LiabilitiesLong, c.LiabilitiesShort, c.NetProfit, c.Income()
	fixed := total * 35 / 100 / 100 * 100
	var tax int64
	if net > 0 {
		tax = net / 4 / 100 * 100 // налог 20% от прибыли до налогообложения
	}
	other := revenue / 100 / 100 * 100
	sales := net + tax + other

	p.bold(20, 22, 13, "Бухгалтерская (финансовая) отчётность субъекта малого предпринимательства")
	p.add(20, 32, 10, "Организация: "+c.Name)
	p.add(20, 38, 10, "ИНН: "+c.INN)
	p.add(20, 44, 10, fmt.Sprintf("Отчётный период: %d год", c.Year))
	p.add(20, 50, 10, "Единица измерения: руб.")

	y := 64.0
	section := func(title, col string, rows [][3]string) {
		p.bold(20, y, 11, title)
		y += 9
		p.bold(20, y, 9.5, "Наименование показателя")
		p.bold(130, y, 9.5, "Код")
		p.texts = append(p.texts, text{x: 190, y: y, size: 9.5, bold: true, right: true, s: col})
		p.rule(y + 2)
		y += 8
		for _, r := range rows {
			p.add(20, y, 9.5, r[0])
			p.add(130, y, 9.5, r[1])
			p.num(190, y, 9.5, r[2])
			y += 6.5
		}
		y += 8
	}
	section(
		fmt.Sprintf("Бухгалтерский баланс на 31 декабря %d г.", c.Year),
		fmt.Sprintf("На 31.12.%d", c.Year),
		[][3]string{
			{"Материальные внеоборотные активы", "1150", rubA(fixed)},
			{"Запасы, дебиторская задолженность, денежные средства", "1230", rubA(total - fixed)},
			{"БАЛАНС (актив)", "1600", rubA(total)},
			{"Капитал и резервы", "1300", rubA(total - long - short)},
			{"Долгосрочные заёмные средства", "1400", rubA(long)},
			{"Краткосрочные обязательства", "1500", rubA(short)},
			{"БАЛАНС (пассив)", "1700", rubA(total)},
		},
	)
	section(
		fmt.Sprintf("Отчёт о финансовых результатах за %d г.", c.Year),
		fmt.Sprintf("За %d г.", c.Year),
		[][3]string{
			{"Выручка", "2110", rubA(revenue)},
			{"Себестоимость продаж", "2120", "(" + rubA(revenue-sales) + ")"},
			{"Прибыль (убыток) от продаж", "2200", signed(sales)},
			{"Прочие расходы", "2350", "(" + rubA(other) + ")"},
			{"Прибыль (убыток) до налогообложения", "2300", signed(net + tax)},
			{"Налог на прибыль", "2410", "(" + rubA(tax) + ")"},
			{"Чистая прибыль (убыток)", "2400", signed(net)},
		},
	)
	p.add(20, 285, 7.5, footer)
	return p
}

var monthNames = [...]string{
	"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
}

func period(c synth.Company) (from, to string) {
	last := c.Months[len(c.Months)-1].Start.AddDate(0, 1, -1)
	return c.Months[0].Start.Format("02.01.2006"), last.Format("02.01.2006")
}

func account(c synth.Company) string {
	prefix := "40702810" // счёт юрлица
	if c.IsIP() {
		prefix = "40802810"
	}
	return prefix + "0000" + c.INN[len(c.INN)-8:]
}

// moneyA: 1 234 567,89
func moneyA(kop int64) string { return groups(kop/100) + fmt.Sprintf(",%02d", abs(kop%100)) }

// moneyB: 1234567.89
func moneyB(kop int64) string {
	return strconv.FormatInt(kop/100, 10) + fmt.Sprintf(".%02d", abs(kop%100))
}

// rubA для отчётности, там целые рубли
func rubA(kop int64) string { return groups(kop / 100) }

// signed: убыток в отчётности пишут в скобках
func signed(kop int64) string {
	if kop < 0 {
		return "(" + rubA(-kop) + ")"
	}
	return rubA(kop)
}

func groups(v int64) string {
	s := strconv.FormatInt(abs(v), 10)
	var b strings.Builder
	if v < 0 {
		b.WriteByte('-')
	}
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
