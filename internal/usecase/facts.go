package usecase

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// Ответы источников, только нужные решению поля. Деньги у источников в рублях.
type egrulReply struct {
	Kind         string
	Status       string
	RegisteredAt string `json:"registered_at"`
	OKVEDMain    struct {
		Code string
	} `json:"okved_main"`
	Founders []struct {
		SharePct  float64 `json:"share_pct"`
		IsForeign bool    `json:"is_foreign"`
	}
	Unreliable []json.RawMessage
}

type fnsReply struct {
	Income  int64 `json:"income_last_year"`
	Arrears struct {
		Tax, Penalty, Fine int64
	}
	MassAddress   bool `json:"mass_address"`
	MassDirector  bool `json:"mass_director"`
	Disqualified  bool `json:"disqualified_director"`
	AccountBlocks []struct {
		ReasonCode int `json:"reason_code"`
	} `json:"account_blocks"`
}

type bkiReply struct {
	Summary struct {
		Payments     int64 `json:"monthly_payments_total"`
		Inquiries90d int   `json:"inquiries_90d"`
		Bankruptcy   bool  `json:"bankruptcy_5y"`
	}
	Loans []struct {
		Role        string
		CurrentDPD  int    `json:"current_dpd"`
		WorstDPD24m string `json:"worst_dpd_24m"`
	}
	Owner struct {
		WorstDPD12m int `json:"worst_dpd_12m"`
		CurrentDPD  int `json:"current_dpd"`
	}
}

type bankReply struct {
	IsClient bool   `json:"is_client"`
	Opened   string `json:"account_opened_at"`
	Turnover []struct {
		Credit int64 `json:"credit"`
	} `json:"turnover_12m"`
	PastLoans   []pastLoan `json:"past_loans"`
	AMLHighRisk bool       `json:"aml_high_risk"`
	// решению не нужны, их показывает профиль
	Account  string   `json:"account"`
	Products []string `json:"products"`
}

type pastLoan struct {
	WorstDPD int `json:"worst_dpd"`
}

const (
	blockCollection  = 1
	blockDeclaration = 2
	// счёт младше 3 мес истории не даёт, с 12 мес это полноценный клиент
	accountNewMonths  = 3
	accountYearMonths = 12
	halfYear          = 6
)

// уровни истории с банком, вход F8, описаны в rules/scoring.yaml
const (
	historyNone = iota
	historyNew
	historyYear
	historyLoan
)

// Facts раскладывает последние ответы источников по фактам. Источник без ответа
// оставляет свои факты нулевыми.
func (u *decisionUseCase) Facts(
	app *domain.Application,
	checks []domain.CheckResult,
	now time.Time,
) (domain.Facts, error) {
	var (
		f    domain.Facts
		eg   egrulReply
		fns  fnsReply
		bki  bkiReply
		bank bankReply
	)
	replies := map[domain.Source]any{
		domain.SourceEGRUL: &eg, domain.SourceFNS: &fns, domain.SourceBKI: &bki, domain.SourceInternal: &bank,
	}
	for _, c := range checks {
		if c.Status != domain.CheckOK {
			continue
		}
		if err := json.Unmarshal(c.Payload, replies[c.Source]); err != nil {
			return f, fmt.Errorf("ответ %s: %w", c.Source, err)
		}
		f.EGRULFound = f.EGRULFound || c.Source == domain.SourceEGRUL
	}

	if err := eg.apply(&f, now); err != nil {
		return f, err
	}
	fns.apply(&f)
	if err := bki.apply(&f); err != nil {
		return f, err
	}
	f.Bankruptcy = f.Bankruptcy || eg.Status == "bankrupt_proc"
	f.AMLHighRisk = bank.AMLHighRisk
	level, err := bank.history(now)
	if err != nil {
		return f, err
	}
	f.BankHistoryLevel = level

	own := make([]int64, 0, len(bank.Turnover))
	for _, t := range bank.Turnover {
		own = append(own, t.Credit*domain.KopInRub)
	}
	u.revenue(&f, app, own, fns.Income*domain.KopInRub, now)
	f.MissingDocuments = u.missingDocs(app, bank.IsClient, mean(own))
	return f, nil
}

func (e *egrulReply) apply(f *domain.Facts, now time.Time) error {
	f.IsIP = e.Kind == "IP"
	f.EGRULStatus = e.Status
	f.OKVEDGroup, _, _ = strings.Cut(e.OKVEDMain.Code, ".")
	f.UnreliableRecords = len(e.Unreliable)
	for _, fd := range e.Founders {
		if fd.IsForeign {
			f.ForeignSharePct += fd.SharePct
		}
	}
	if !f.EGRULFound {
		return nil
	}
	reg, err := time.Parse(time.DateOnly, e.RegisteredAt)
	if err != nil {
		return fmt.Errorf("егрюл registered_at: %w", err)
	}
	f.BusinessAgeMonths = monthsSince(reg, now)
	return nil
}

func (r *fnsReply) apply(f *domain.Facts) {
	f.DisqualifiedDirector, f.MassAddress, f.MassDirector = r.Disqualified, r.MassAddress, r.MassDirector
	f.TaxArrearsKop = (r.Arrears.Tax + r.Arrears.Penalty + r.Arrears.Fine) * domain.KopInRub
	for _, b := range r.AccountBlocks {
		f.TaxBlockCollection = f.TaxBlockCollection || b.ReasonCode == blockCollection
		f.TaxBlockDeclaration = f.TaxBlockDeclaration || b.ReasonCode == blockDeclaration
	}
}

func (b *bkiReply) apply(f *domain.Facts) error {
	f.BKIPaymentsKop = b.Summary.Payments * domain.KopInRub
	f.Inquiries90d = b.Summary.Inquiries90d
	f.Bankruptcy = b.Summary.Bankruptcy
	f.OwnerWorstDPD12m = b.Owner.WorstDPD12m
	f.CurrentDPDMax = b.Owner.CurrentDPD
	for _, l := range b.Loans {
		f.CurrentDPDMax = max(f.CurrentDPDMax, l.CurrentDPD)
		if l.Role != "borrower" {
			continue
		}
		// бины бюро "0", "1-29", "30-59", "60-89", "90+", берём нижнюю границу
		lo, _, _ := strings.Cut(strings.TrimSuffix(l.WorstDPD24m, "+"), "-")
		dpd, err := strconv.Atoi(lo)
		if err != nil {
			return fmt.Errorf("бки worst_dpd_24m %q: %w", l.WorstDPD24m, err)
		}
		f.WorstDPD24m = max(f.WorstDPD24m, dpd)
	}
	return nil
}

func (b *bankReply) history(now time.Time) (int, error) {
	if !b.IsClient {
		return historyNone, nil
	}
	opened, err := time.Parse(time.DateOnly, b.Opened)
	if err != nil {
		return historyNone, fmt.Errorf("банк account_opened_at: %w", err)
	}
	switch age := monthsSince(opened, now); {
	case age < accountNewMonths:
		return historyNone, nil
	case age < accountYearMonths:
		return historyNew, nil
	case slices.ContainsFunc(b.PastLoans, func(l pastLoan) bool { return l.WorstDPD == 0 }):
		return historyLoan, nil
	}
	return historyYear, nil
}

// revenue считает выручку и обороты. Помесячный ряд берём с нашего счёта, без него
// из подтверждённой выписки. M консервативно: меньшее из ФНС / 12 и оборота (4.4).
func (u *decisionUseCase) revenue(
	f *domain.Facts,
	app *domain.Application,
	series []int64,
	fnsKop int64,
	now time.Time,
) {
	if len(series) == 0 {
		series = statement(app.Documents)
	}
	t := mean(series)
	f.MonthlyTurnoverKop = t
	f.RevenueTrend = 1 // по ряду короче года тренд не видно, ставим нейтральный
	if n := len(series); n >= 2*halfYear {
		f.RevenueTrend = float64(sum(series[n-halfYear:])) / float64(sum(series[n-2*halfYear:n-halfYear]))
	}
	if t > 0 {
		var sq float64
		for _, v := range series {
			d := float64(v - t)
			sq += d * d
		}
		f.TurnoverCV = math.Sqrt(sq/float64(len(series))) / float64(t)
	}

	fns := fnsKop / accountYearMonths
	var avail []int64
	for _, v := range []int64{fns, t} {
		if v > 0 {
			avail = append(avail, v)
		}
	}
	f.MonthlyRevenueKop = app.MonthlyRevenueKop
	if len(avail) > 0 {
		f.MonthlyRevenueKop = slices.Min(avail)
	}
	if app.MonthlyRevenueKop > 0 && fns > 0 {
		f.RevenueMismatch = math.Abs(float64(app.MonthlyRevenueKop-fns)) / float64(fns)
	}

	var limit float64
	for _, r := range u.policy.StopFactors {
		if r.Code == domain.CodeDocMismatch {
			limit, _ = r.Value.(float64)
		}
	}
	fy := &domain.FNSIncome{Year: now.Year() - 1, Kop: fnsKop}
	rc := domain.CheckRevenue(app.Documents, fy, limit)
	f.DocRevenueMismatch, f.DocRevenueKop, f.FNSRevenueKop = rc.Mismatch, rc.DocKop, rc.FNSKop
}

// missingDocs по правилам продукта. Выписку засчитываем только подтверждённой: из неё берём
// обороты. Отчётности хватает файла, декларацию ИП мы и не распознаём
func (u *decisionUseCase) missingDocs(app *domain.Application, client bool, ownKop int64) []string {
	p, _ := u.catalog.Product(app.Product)
	var need []string
	for _, d := range p.Documents {
		switch d.When {
		case "no_turnover":
			enough := app.MonthlyRevenueKop == 0 ||
				float64(ownKop) >= u.catalog.OwnTurnoverMinShare*float64(app.MonthlyRevenueKop)
			if client && enough {
				continue
			}
		case "amount_over":
			if app.AmountKop <= d.AmountOverKop {
				continue
			}
		}
		need = append(need, d.Kind)
	}
	if app.Role == "proxy" {
		need = append(need, u.catalog.ProxyDocument)
	}
	return slices.DeleteFunc(need, func(kind string) bool {
		return slices.ContainsFunc(app.Documents, func(d domain.Document) bool {
			return string(d.Kind) == kind && (d.Kind != domain.DocStatement || d.Status == domain.DocConfirmed)
		})
	})
}

func statement(docs []domain.Document) []int64 {
	for i := range docs {
		d := &docs[i]
		if d.Status != domain.DocConfirmed || d.Confirmed == nil || d.Confirmed.Statement == nil {
			continue
		}
		var out []int64
		for _, m := range d.Confirmed.Statement.MonthlyIn {
			out = append(out, m.Amount)
		}
		return out
	}
	return nil
}

// monthsSince считает полные месяцы: с 15.06 по 14.07 это 0, по 15.07 уже 1
func monthsSince(from, now time.Time) int {
	m := (now.Year()-from.Year())*accountYearMonths + int(now.Month()-from.Month())
	if now.Day() < from.Day() {
		m--
	}
	return m
}

func sum(s []int64) int64 {
	var t int64
	for _, v := range s {
		t += v
	}
	return t
}

func mean(s []int64) int64 {
	if len(s) == 0 {
		return 0
	}
	return sum(s) / int64(len(s))
}
