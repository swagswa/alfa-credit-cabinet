package docgen

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"codeberg.org/go-pdf/fpdf"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// Что попадает в договор: условия из решения и реквизиты заявки.
type Contract struct {
	Number int64
	Date   time.Time
	// Product название продукта для текста, пустое: не пишем
	Product   string
	Company   string
	INN       string
	Terms     domain.Terms
	Repayment domain.Repayment
}

// Payment строка графика, суммы в копейках.
type Payment struct {
	Month        int
	PaymentKop   int64
	InterestKop  int64
	PrincipalKop int64
	BalanceKop   int64
}

// Schedule раскладывает аннуитет по месяцам. Платёж берём из условий, его видел клиент,
// последний месяц добирает остаток, чтобы округления не оставили копейки долга.
func Schedule(t domain.Terms) []Payment {
	rows := make([]Payment, t.TermMonths)
	bal := t.AmountKop
	for i := range rows {
		interest := int64(math.Round(float64(bal) * float64(t.RateBP) / 10000 / 12))
		principal := t.PaymentKop - interest
		if i == len(rows)-1 {
			principal = bal
		}
		bal -= principal
		rows[i] = Payment{
			Month:        i + 1,
			PaymentKop:   interest + principal,
			InterestKop:  interest,
			PrincipalKop: principal,
			BalanceKop:   bal,
		}
	}
	return rows
}

// ContractPDF собирает договор с графиком в приложении. Один и тот же Contract даёт
// байт в байт тот же файл: шаг процесса может повториться.
func ContractPDF(c Contract) ([]byte, error) {
	f := newPDF()
	f.AddUTF8FontFromBytes("pt", "", regularTTF)
	f.AddUTF8FontFromBytes("pt", "B", boldTTF)
	f.SetMargins(left, 20, 210-right)
	f.AddPage()

	t := c.Terms
	f.SetFont("pt", "B", 14)
	f.CellFormat(0, 8, fmt.Sprintf("Кредитный договор № %d", c.Number), "", 1, "C", false, 0, "")
	f.SetFont("pt", "", 10)
	f.CellFormat(0, 6, "г. Москва, "+c.Date.Format("02.01.2006"), "", 1, "R", false, 0, "")
	f.Ln(4)

	para(f, "АО «Альфа-Банк» (далее Банк) и "+c.Company+", ИНН "+c.INN+
		" (далее Заёмщик), заключили договор о нижеследующем.")
	product := ""
	if c.Product != "" {
		product = " по продукту «" + c.Product + "»"
	}
	para(f, fmt.Sprintf("1. Банк предоставляет Заёмщику кредит%s в сумме %s руб. на %d мес. под %s %% годовых.",
		product, Rub(t.AmountKop).Text(" ", ","), t.TermMonths, Rub(t.RateBP).Text("", ",")))
	if c.Repayment == domain.AutoDebit {
		para(f, "2. Кредит выдаётся траншами в пределах лимита, каждый транш до 60 дней. Задолженность "+
			"погашается списанием поступлений на расчётный счёт Заёмщика в Банке, проценты начисляются "+
			"на фактическую задолженность.")
	} else {
		para(f, fmt.Sprintf("2. Заёмщик погашает кредит ежемесячными равными платежами по %s руб. "+
			"по графику в приложении 1.", Rub(t.PaymentKop).Text(" ", ",")))
	}
	var extra []string
	if t.Guarantor {
		extra = append(extra, "поручительство собственника бизнеса")
	}
	if t.Pledge {
		extra = append(extra, "залог приобретаемого имущества")
	}
	if len(extra) > 0 {
		para(f, "3. Обеспечение: "+strings.Join(extra, ", ")+".")
	}
	para(f, "Договор подписывается простой электронной подписью: кодом подтверждения в кабинете Заёмщика.")
	f.Ln(6)
	f.CellFormat(85, 6, "Банк: ____________________", "", 0, "L", false, 0, "")
	f.CellFormat(0, 6, "Заёмщик: ____________________", "", 1, "L", false, 0, "")

	f.SetFont("pt", "", 7.5)
	f.SetY(285 - 4)
	f.CellFormat(0, 4, "Прототип Alfa-Hack 2026, синтетический документ, не является договором реального банка.",
		"", 0, "L", false, 0, "")

	if c.Repayment != domain.AutoDebit {
		schedule(f, Schedule(t))
	}

	return output(f)
}

func para(f *fpdf.Fpdf, s string) {
	f.MultiCell(0, 5.5, s, "", "J", false)
	f.Ln(2)
}

// schedule рисует график с новой страницы, шапка повторяется на каждой
func schedule(f *fpdf.Fpdf, rows []Payment) {
	cols := []struct {
		title string
		w     float64
	}{{"№", 14}, {"Платёж", 39}, {"Проценты", 39}, {"Основной долг", 39}, {"Остаток", 39}}
	head := func() {
		f.SetFont("pt", "B", 9)
		for _, c := range cols {
			f.CellFormat(c.w, 7, c.title, "1", 0, "C", false, 0, "")
		}
		f.Ln(-1)
		f.SetFont("pt", "", 9)
	}

	f.AddPage()
	f.SetFont("pt", "B", 12)
	f.CellFormat(0, 8, "Приложение 1. График платежей", "", 1, "L", false, 0, "")
	head()
	for _, r := range rows {
		if f.GetY() > 270 {
			f.AddPage()
			head()
		}
		cells := []string{
			strconv.Itoa(r.Month), Rub(r.PaymentKop).Text(" ", ","), Rub(r.InterestKop).Text(" ", ","),
			Rub(r.PrincipalKop).Text(" ", ","), Rub(r.BalanceKop).Text(" ", ","),
		}
		for i, s := range cells {
			f.CellFormat(cols[i].w, 6, s, "1", 0, "R", false, 0, "")
		}
		f.Ln(-1)
	}
}
