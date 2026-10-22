package docgen

import (
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
	"github.com/stretchr/testify/require"

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
	for _, tc := range []struct {
		doc    Doc
		in     int64    // поступления за год по выписке, руб, ±12 на округление месяцев
		ratio  float64  // во сколько раз выписка больше выручки в ФНС
		phrase []string // что должно читаться из текстового слоя
	}{
		{Doc{INN: "0000100012", Kind: KindStatement, Format: "a"}, 14_400_000, 1, []string{"Выписка по расчётному счёту", "Обороты по кредиту", "ООО «Кофейня на Садовой»"}},
		{Doc{INN: "000020001010", Kind: KindStatement, Format: "b"}, 24_000_000, 1, []string{"ВЫПИСКА ОПЕРАЦИЙ ПО СЧЁТУ", "Поступления:", "Сентябрь 2026"}},
		{Doc{INN: "0000300011", Kind: KindStatement, Format: "a"}, 42_000_000, 1, []string{"ИНН: 0000300011", "Исходящий остаток"}},
		{Doc{INN: "0000300011", Kind: KindStatement, Format: "a", Trap: true}, 63_000_000, 1.5, []string{"ИНН: 0000300011"}},
		{Doc{INN: "0000400016", Kind: KindStatement, Format: "c"}, 48_000_000, 1, nil},
		{Doc{INN: "0000100012", Kind: KindStatement, Format: "c", PDF: true}, 14_400_000, 1, nil},
	} {
		t.Run(tc.doc.Name()+tc.doc.Ext(), func(t *testing.T) {
			file, raw, err := Generate(tc.doc)
			require.NoError(t, err)
			var s Statement
			require.NoError(t, json.Unmarshal(raw, &s))

			require.Equal(t, tc.doc.INN, s.INN)
			require.Equal(t, "2025-10-01", s.PeriodFrom)
			require.Equal(t, "2026-09-30", s.PeriodTo)
			require.Len(t, s.MonthlyIn, 12)
			require.Equal(t, "2025-10", s.MonthlyIn[0].Month)
			var sum Rub
			for _, m := range s.MonthlyIn {
				sum += m.Amount
			}
			require.Equal(t, s.TurnoverIn, sum)
			require.Equal(t, s.ClosingBalance, s.OpeningBalance+s.TurnoverIn-s.TurnoverOut)
			require.InDelta(t, tc.in*100, int64(s.TurnoverIn), 18*100)

			// сверка с ФНС как в 06 раздел 8.5: у честной выписки совпадает, у ловушки нет
			c, err := synth.New(tc.doc.INN)
			require.NoError(t, err)
			require.InDelta(t, tc.ratio, float64(s.TurnoverIn)/float64(c.Income()), 0.001)

			switch {
			case tc.doc.Ext() == ".png":
				img, err := png.Decode(bytes.NewReader(file))
				require.NoError(t, err)
				require.Equal(t, 1240, img.Bounds().Dx())
				require.Equal(t, 1754, img.Bounds().Dy())
			case tc.doc.Format == "c":
				require.Empty(t, strings.TrimSpace(pdfText(t, file)), "у скана нет текстового слоя")
			default:
				txt := pdfText(t, file)
				for _, p := range tc.phrase {
					require.Contains(t, txt, p)
				}
			}
		})
	}
}

func TestFinancials(t *testing.T) {
	for _, tc := range []struct {
		inn     string
		revenue int64 // руб
	}{
		{"0000100012", 14_400_000},
		{"0000400016", 48_000_000},
	} {
		t.Run(tc.inn, func(t *testing.T) {
			file, raw, err := Generate(Doc{INN: tc.inn, Kind: KindFinancials, Format: "a"})
			require.NoError(t, err)
			var f Financials
			require.NoError(t, json.Unmarshal(raw, &f))
			require.Equal(t, tc.inn, f.INN)
			require.Equal(t, 2025, f.Year)
			require.InDelta(t, tc.revenue*100, int64(f.Revenue), 18*100)
			require.GreaterOrEqual(t, f.BalanceTotal, f.LiabilitiesLong+f.LiabilitiesShort)

			c, err := synth.New(tc.inn)
			require.NoError(t, err)
			require.Equal(t, Rub(c.Income()), f.Revenue, "выручка в отчётности равна выручке в ФНС")

			txt := pdfText(t, file)
			for _, p := range []string{"Бухгалтерский баланс", "Выручка", "2110", "1600", "ИНН: " + tc.inn} {
				require.Contains(t, txt, p)
			}
		})
	}
}

func TestGenerateDeterministic(t *testing.T) {
	for _, d := range []Doc{
		{INN: "0012345673", Kind: KindStatement, Format: "a"},
		{INN: "0012345673", Kind: KindStatement, Format: "b"},
		{INN: "0012345673", Kind: KindStatement, Format: "c"},
		{INN: "001234567887", Kind: KindStatement, Format: "c", PDF: true},
		{INN: "0012345673", Kind: KindFinancials, Format: "a"},
	} {
		t.Run(d.Name()+d.Ext(), func(t *testing.T) {
			f1, t1, err := Generate(d)
			require.NoError(t, err)
			f2, t2, err := Generate(d)
			require.NoError(t, err)
			require.Equal(t, t1, t2)
			require.True(t, bytes.Equal(f1, f2), "файл отличается между запусками")
		})
	}
}

func TestGenerateErrors(t *testing.T) {
	for _, d := range []Doc{
		{INN: "0000100013", Kind: KindStatement, Format: "a"},
		{INN: "", Kind: KindStatement, Format: "a"},
		{INN: "0000100012", Kind: KindStatement, Format: "d"},
		{INN: "0000100012", Kind: "passport", Format: "a"},
		{INN: "000020001010", Kind: KindFinancials, Format: "a"},
		{INN: "0000100012", Kind: KindFinancials, Format: "b"},
		{INN: "0000300011", Kind: KindFinancials, Format: "a", Trap: true},
	} {
		t.Run(d.Name(), func(t *testing.T) {
			_, _, err := Generate(d)
			require.Error(t, err)
		})
	}
}

func TestRubJSON(t *testing.T) {
	for _, tc := range []struct {
		v    Rub
		json string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{-150, "-1.50"},
		{123456789, "1234567.89"},
		{-4583868_00, "-4583868.00"},
	} {
		t.Run(tc.json, func(t *testing.T) {
			b, err := json.Marshal(tc.v)
			require.NoError(t, err)
			require.JSONEq(t, tc.json, string(b))
			var back Rub
			require.NoError(t, json.Unmarshal(b, &back))
			require.Equal(t, tc.v, back)
		})
	}
}
