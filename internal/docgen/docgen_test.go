package docgen_test

import (
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/mocks/synth"
)

// pdfText достаёт текст той же библиотекой, что и распознавание
func pdfText(t *testing.T, b []byte) string {
	t.Helper()
	r, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	require.NoError(t, err)
	txt, err := r.GetPlainText()
	require.NoError(t, err)
	s, err := io.ReadAll(txt)
	require.NoError(t, err)
	return string(s)
}

func TestStatement(t *testing.T) {
	tests := []struct {
		doc    docgen.Doc // Kind проставляется в цикле, тут везде выписка
		in     int64      // поступления за год по выписке, руб, допуск 18 руб на округление месяцев
		ratio  float64    // во сколько раз выписка больше выручки в ФНС
		phrase []string   // что должно читаться из текстового слоя
	}{
		{docgen.Doc{INN: "0000100012", Format: "a"}, 14_400_000, 1,
			[]string{"Выписка по расчётному счёту", "Обороты по кредиту", "ООО «Тёплое зерно»"}},
		{docgen.Doc{INN: "000020001010", Format: "b"}, 24_000_000, 1,
			[]string{"ВЫПИСКА ОПЕРАЦИЙ ПО СЧЁТУ", "Поступления:", "Сентябрь 2026"}},
		{docgen.Doc{INN: "0000300011", Format: "a"}, 42_000_000, 1, []string{"ИНН: 0000300011", "Исходящий остаток"}},
		{docgen.Doc{INN: "0000300011", Format: "a", Trap: true}, 63_000_000, 1.5, []string{"ИНН: 0000300011"}},
		{docgen.Doc{INN: "0000400016", Format: "c"}, 48_000_000, 1, nil},
		{docgen.Doc{INN: "0000100012", Format: "c", PDF: true}, 14_400_000, 1, nil},
	}
	for _, tt := range tests {
		tt.doc.Kind = docgen.KindStatement
		t.Run(tt.doc.Name()+tt.doc.Ext(), func(t *testing.T) {
			file, raw, err := docgen.Generate(tt.doc)
			require.NoError(t, err)
			var s docgen.Statement
			require.NoError(t, json.Unmarshal(raw, &s))

			require.Equal(t, tt.doc.INN, s.INN)
			require.Equal(t, "2025-10-01", s.PeriodFrom)
			require.Equal(t, "2026-09-30", s.PeriodTo)
			require.Len(t, s.MonthlyIn, 12)
			require.Equal(t, "2025-10", s.MonthlyIn[0].Month)
			var sum docgen.Rub
			for _, m := range s.MonthlyIn {
				sum += m.Amount
			}
			require.Equal(t, s.TurnoverIn, sum)
			require.Equal(t, s.ClosingBalance, s.OpeningBalance+s.TurnoverIn-s.TurnoverOut)
			require.InDelta(t, tt.in*100, int64(s.TurnoverIn), 18*100)

			// сверка с выручкой в ФНС: у честной выписки совпадает, у ловушки нет
			c, err := synth.New(tt.doc.INN)
			require.NoError(t, err)
			require.InDelta(t, tt.ratio, float64(s.TurnoverIn)/float64(c.Income()), 0.001)

			switch {
			case tt.doc.Ext() == ".png":
				img, err := png.Decode(bytes.NewReader(file))
				require.NoError(t, err)
				require.Equal(t, 1240, img.Bounds().Dx())
				require.Equal(t, 1754, img.Bounds().Dy())
			case tt.doc.Format == "c":
				require.Empty(t, strings.TrimSpace(pdfText(t, file)), "у скана нет текстового слоя")
			default:
				txt := pdfText(t, file)
				for _, p := range tt.phrase {
					require.Contains(t, txt, p)
				}
			}
		})
	}
}

func TestFinancials(t *testing.T) {
	tests := []struct {
		inn     string
		revenue int64 // руб
	}{
		{"0000100012", 14_400_000},
		{"0000400016", 48_000_000},
	}
	for _, tt := range tests {
		t.Run(tt.inn, func(t *testing.T) {
			file, raw, err := docgen.Generate(docgen.Doc{INN: tt.inn, Kind: docgen.KindFinancials, Format: "a"})
			require.NoError(t, err)
			var f docgen.Financials
			require.NoError(t, json.Unmarshal(raw, &f))
			require.Equal(t, tt.inn, f.INN)
			require.Equal(t, 2025, f.Year)
			require.InDelta(t, tt.revenue*100, int64(f.Revenue), 18*100)
			require.GreaterOrEqual(t, f.BalanceTotal, f.LiabilitiesLong+f.LiabilitiesShort)

			c, err := synth.New(tt.inn)
			require.NoError(t, err)
			require.Equal(t, docgen.Rub(c.Income()), f.Revenue, "выручка в отчётности равна выручке в ФНС")

			txt := pdfText(t, file)
			for _, p := range []string{"Бухгалтерский баланс", "Выручка", "2110", "1600", "ИНН: " + tt.inn} {
				require.Contains(t, txt, p)
			}
		})
	}
}

func TestGenerateDeterministic(t *testing.T) {
	for _, d := range []docgen.Doc{
		{INN: "0012345673", Kind: docgen.KindStatement, Format: "a"},
		{INN: "0012345673", Kind: docgen.KindStatement, Format: "b"},
		{INN: "0012345673", Kind: docgen.KindStatement, Format: "c"},
		{INN: "001234567887", Kind: docgen.KindStatement, Format: "c", PDF: true},
		{INN: "0012345673", Kind: docgen.KindFinancials, Format: "a"},
	} {
		t.Run(d.Name()+d.Ext(), func(t *testing.T) {
			f1, t1, err := docgen.Generate(d)
			require.NoError(t, err)
			f2, t2, err := docgen.Generate(d)
			require.NoError(t, err)
			require.Equal(t, t1, t2)
			require.True(t, bytes.Equal(f1, f2), "файл отличается между запусками")
		})
	}
}

func TestGenerateErrors(t *testing.T) {
	for _, d := range []docgen.Doc{
		{INN: "0000100013", Kind: docgen.KindStatement, Format: "a"},
		{INN: "", Kind: docgen.KindStatement, Format: "a"},
		{INN: "0000100012", Kind: docgen.KindStatement, Format: "d"},
		{INN: "0000100012", Kind: "passport", Format: "a"},
		{INN: "000020001010", Kind: docgen.KindFinancials, Format: "a"},
		{INN: "0000100012", Kind: docgen.KindFinancials, Format: "b"},
		{INN: "0000300011", Kind: docgen.KindFinancials, Format: "a", Trap: true},
	} {
		t.Run(d.Name(), func(t *testing.T) {
			_, _, err := docgen.Generate(d)
			require.Error(t, err)
		})
	}
}

func TestRub(t *testing.T) {
	tests := []struct {
		v    docgen.Rub
		json string // он же формат B
		a    string
	}{
		{0, "0.00", "0,00"},
		{5, "0.05", "0,05"},
		{-1, "-0.01", "-0,01"},
		{-50, "-0.50", "-0,50"},
		{-99, "-0.99", "-0,99"},
		{-150, "-1.50", "-1,50"},
		{123456789, "1234567.89", "1 234 567,89"},
		{-4583868_00, "-4583868.00", "-4 583 868,00"},
	}
	for _, tt := range tests {
		t.Run(tt.json, func(t *testing.T) {
			require.Equal(t, tt.json, tt.v.Text("", "."))
			require.Equal(t, tt.a, tt.v.Text(" ", ","))
			b, err := json.Marshal(tt.v)
			require.NoError(t, err)
			require.JSONEq(t, tt.json, string(b))
			var back docgen.Rub
			require.NoError(t, json.Unmarshal(b, &back))
			require.Equal(t, tt.v, back)
		})
	}
}
