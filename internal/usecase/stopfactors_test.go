package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

func TestCheck(t *testing.T) {
	uc := decisions(t)
	// M = 1 млн, остальное чисто
	clean := func(edit func(f *domain.Facts)) domain.Facts {
		f := domain.Facts{
			EGRULFound:        true,
			EGRULStatus:       "active",
			BusinessAgeMonths: 40,
			MonthlyRevenueKop: 100_000_000,
		}
		if edit != nil {
			edit(&f)
		}
		return f
	}
	tests := []struct {
		name    string
		facts   domain.Facts
		outcome domain.Status
		codes   []string
		text    string // текст первой причины
	}{
		{name: "чисто", facts: clean(nil)},
		{
			name:    "ликвидация",
			facts:   clean(func(f *domain.Facts) { f.EGRULStatus = "liquidating" }),
			outcome: domain.StatusRejected,
			codes:   []string{"SF_STATUS_INACTIVE"},
			text:    "По данным ЕГРЮЛ компания не действует (ликвидация или реорганизация)",
		},
		{
			name: "декларация и массовый адрес",
			facts: clean(func(f *domain.Facts) {
				f.TaxBlockDeclaration = true
				f.MassAddress = true
			}),
			outcome: domain.StatusNeedInfo,
			codes:   []string{"SF_TAX_BLOCK_DECL", "SF_MASS_ADDRESS"},
			text:    "Налоговая приостановила операции из-за несданной декларации. Загрузите квитанцию о сдаче",
		},
		{name: "недоимка 6% выручки", facts: clean(func(f *domain.Facts) { f.TaxArrearsKop = 6_000_000 })},
		{
			name:    "недоимка 11% выручки",
			facts:   clean(func(f *domain.Facts) { f.TaxArrearsKop = 11_000_000 }),
			outcome: domain.StatusRejected,
			codes:   []string{"SF_TAX_ARREARS"},
			text:    "Есть налоговая задолженность. Погасите её и подайте заявку снова",
		},
		{
			name: "документ не сходится с ФНС",
			facts: clean(func(f *domain.Facts) {
				f.DocRevenueMismatch = 0.5
				f.DocRevenueKop, f.FNSRevenueKop = 6_299_999_850, 4_199_999_900
			}),
			codes: []string{"SF_DOC_MISMATCH"},
			text: "Цифры в документе расходятся с данными налоговой на 50%: выручка за год по документу " +
				"62 999 998 ₽, у налоговой 41 999 999 ₽. Заявку посмотрит специалист",
		},
		{
			name: "отказ, запрос и ручное вместе",
			facts: clean(func(f *domain.Facts) {
				f.Bankruptcy = true
				f.RevenueMismatch = 0.5
				f.AMLHighRisk = true
			}),
			outcome: domain.StatusRejected,
			codes:   []string{"SF_BANKRUPTCY", "SF_REVENUE_MISMATCH", "SF_AML_FLAG"},
			text:    "По данным реестров у компании есть процедура банкротства или банкротство за последние 5 лет",
		},
		{
			name:    "нет выписки",
			facts:   clean(func(f *domain.Facts) { f.MissingDocuments = []string{"statement"} }),
			outcome: domain.StatusNeedInfo,
			codes:   []string{"SF_DOCS_MISSING"},
			text:    "Не хватает документа: выписка по счёту из другого банка",
		},
		{
			name:    "нет двух документов",
			facts:   clean(func(f *domain.Facts) { f.MissingDocuments = []string{"statement", "financials"} }),
			outcome: domain.StatusNeedInfo,
			codes:   []string{"SF_DOCS_MISSING"},
			text:    "Не хватает документа: выписка по счёту из другого банка, бухгалтерская отчётность или декларация",
		},
		{
			name:    "текущая просрочка 95 дней",
			facts:   clean(func(f *domain.Facts) { f.CurrentDPDMax = 95 }),
			outcome: domain.StatusRejected,
			codes:   []string{"SF_OVERDUE_90_ACTIVE"},
			text:    "Есть текущая просрочка больше 90 дней. После погашения можно подать заявку снова",
		},
		{
			name:  "мягкий без текста клиенту",
			facts: clean(func(f *domain.Facts) { f.MassDirector = true }),
			codes: []string{"SF_MASS_DIRECTOR"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := uc.Check(&tt.facts)

			require.Equal(t, domain.StageChecks, d.Stage)
			require.Equal(t, "2026-10-21.1", d.PolicyVersion)
			require.Equal(t, tt.outcome, d.Outcome)
			var hits []string
			for _, h := range d.Hits {
				hits = append(hits, h.Code)
			}
			require.Equal(t, tt.codes, hits)
			require.Len(t, d.Reasons, len(hits))
			require.Len(t, d.Documents, len(tt.facts.MissingDocuments))
			if len(tt.codes) > 0 {
				require.Equal(t, tt.text, d.Reasons[0].Text)
			}
		})
	}
}
