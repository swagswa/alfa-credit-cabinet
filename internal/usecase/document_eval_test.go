//go:build eval

package usecase_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/config"
	"github.com/swagswa/alfa-credit-cabinet/internal/docgen"
	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ocr"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ollama"
	"github.com/swagswa/alfa-credit-cabinet/internal/usecase"
)

const (
	docsDir = "../../testdata/documents"
	report  = "../../docs/ai-eval.md"
	manual  = "<!-- ниже руками, eval-docs не трогает -->"
)

// цель по точности полей на формат, в отчёте docs/ai-eval.md колонка «Цель по полям»
var target = map[string]float64{"a": 0.95, "b": 0.95, "c": 0.85, "ловушка": 0.95}

type field struct {
	name string
	ok   bool
}

type result struct {
	file, format string
	rec          domain.Recognition
	failed       bool // проверка в Go не пропустила
	down         bool // модель или OCR не ответили вовремя
	fields       []field
}

func (r *result) correct() int {
	n := 0
	for _, f := range r.fields {
		if f.ok {
			n++
		}
	}
	return n
}

// TestDocsEval гоняет testdata/documents через весь конвейер и пишет docs/ai-eval.md.
// Нужны tesseract и Ollama с моделью, адреса как у docs-worker (config.Recognition).
func TestDocsEval(t *testing.T) {
	cfg, err := config.Load[config.Recognition]()
	require.NoError(t, err)
	bin, err := exec.LookPath(cfg.Tesseract)
	if err != nil {
		t.Skipf("нет tesseract: %v", err)
	}
	llm, err := ollama.New(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaTimeout)
	require.NoError(t, err)
	digest, err := llm.Digest(t.Context())
	if err != nil {
		t.Skipf("нет модели: %v", err)
	}
	uc := usecase.NewDocument(nil, nil, nil, ocr.New(bin, cfg.OCRTimeout), llm, true)

	files, err := filepath.Glob(filepath.Join(docsDir, cmp.Or(os.Getenv("EVAL_GLOB"), "*_*_*.*")))
	require.NoError(t, err)
	files = slices.DeleteFunc(files, func(f string) bool { return strings.HasSuffix(f, ".json") })
	require.NotEmpty(t, files)

	var results []result
	for _, path := range files {
		r := read(t, uc, path)
		t.Logf("%s: %d из %d полей, failed=%v", r.file, r.correct(), len(r.fields), r.failed)
		results = append(results, r)
	}

	md := render(results, cfg, digest, tesseractVersion(bin), offload(cfg.OllamaURL, cfg.OllamaModel))
	t.Log("\n" + md)
	// часть набора (EVAL_GLOB) только в лог: отчёт пишем по полному прогону
	if os.Getenv("EVAL_GLOB") != "" {
		return
	}
	// всё после маркера пишется руками (замеры на другом железе), его сохраняем
	if old, err := os.ReadFile(report); err == nil {
		if i := bytes.Index(old, []byte(manual)); i >= 0 {
			md += "\n" + string(old[i:])
		}
	}
	require.NoError(t, os.WriteFile(report, []byte(md), 0o644))
}

func read(t *testing.T, uc usecase.DocumentUseCase, path string) result {
	t.Helper()
	file := filepath.Base(path)
	name := strings.TrimSuffix(file, filepath.Ext(file))
	parts := strings.Split(name, "_") // inn_kind_format[_trap]
	inn, kind := parts[0], domain.DocKind(parts[1])
	format := parts[2]
	if len(parts) > 3 {
		format = "ловушка"
	}
	ct := "application/pdf"
	if filepath.Ext(file) == ".png" {
		ct = "image/png"
	}

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	truth, err := os.ReadFile(filepath.Join(docsDir, name+".truth.json"))
	require.NoError(t, err)

	// таймаут модели не роняем весь прогон: в docs-worker такой документ после
	// ретраев тоже уйдёт в failed, а клиент введёт итоги руками
	rec, err := uc.Read(t.Context(), kind, ct, content, inn)
	r := result{
		file:   file,
		format: format,
		rec:    rec,
		failed: err != nil,
		down:   err != nil && !errors.Is(err, domain.ErrInvalid),
	}
	switch kind {
	case domain.DocStatement:
		var want docgen.Statement
		require.NoError(t, json.Unmarshal(truth, &want))
		r.fields = statementFields(cmp.Or(rec.Values.Statement, &domain.Statement{}), &want)
	case domain.DocFinancials:
		var want docgen.Financials
		require.NoError(t, json.Unmarshal(truth, &want))
		r.fields = financialsFields(cmp.Or(rec.Values.Financials, &domain.Financials{}), &want)
	}
	return r
}

// суммы до рубля, как допуск в проверках
func near(got int64, want docgen.Rub) bool { return max(got-int64(want), int64(want)-got) <= 100 }

func statementFields(got *domain.Statement, want *docgen.Statement) []field {
	months := len(got.MonthlyIn) == len(want.MonthlyIn)
	for i := range min(len(got.MonthlyIn), len(want.MonthlyIn)) {
		g, w := got.MonthlyIn[i], want.MonthlyIn[i]
		months = months && g.Month == w.Month && near(g.Amount, w.Amount)
	}
	return []field{
		{"inn", got.INN == want.INN},
		{"period_from", got.PeriodFrom == want.PeriodFrom},
		{"period_to", got.PeriodTo == want.PeriodTo},
		{"opening_balance", near(got.OpeningBalance, want.OpeningBalance)},
		{"turnover_in", near(got.TurnoverIn, want.TurnoverIn)},
		{"turnover_out", near(got.TurnoverOut, want.TurnoverOut)},
		{"closing_balance", near(got.ClosingBalance, want.ClosingBalance)},
		{"monthly_in", months},
	}
}

func financialsFields(got *domain.Financials, want *docgen.Financials) []field {
	return []field{
		{"inn", got.INN == want.INN},
		{"year", got.Year == want.Year},
		{"revenue", near(got.Revenue, want.Revenue)},
		{"net_profit", near(got.NetProfit, want.NetProfit)},
		{"liabilities_long", near(got.LiabilitiesLong, want.LiabilitiesLong)},
		{"liabilities_short", near(got.LiabilitiesShort, want.LiabilitiesShort)},
		{"balance_total", near(got.BalanceTotal, want.BalanceTotal)},
	}
}

type md struct{ strings.Builder }

func (b *md) p(format string, args ...any) { fmt.Fprintf(b, format+"\n", args...) }

func render(results []result, cfg config.Recognition, digest, tess, hw string) string {
	var b md
	b.p("# Точность распознавания документов")
	b.p("")
	b.p("Пишет `make eval-docs`, руками не правим. Каждый файл из `testdata/documents` проходит весь конвейер")
	b.p("(текстовый слой PDF или Tesseract, модель, проверки в Go) и сравнивается с эталоном `*.truth.json`.")
	b.p("Суммы считаем верными с точностью до 1 ₽, даты, год и ИНН точно,")
	b.p("`monthly_in` верен, только если сошлись все месяцы.")
	b.p("Перед сменой модели, промпта или схемы прогон повторяем и коммитим новый файл.")
	b.p("")
	b.p("| | |")
	b.p("|---|---|")
	b.p("| Дата прогона | %s |", time.Now().Format(time.DateOnly))
	b.p("| Модель | `%s` |", cfg.OllamaModel)
	b.p("| Digest | `%s` |", digest)
	b.p("| Версия конвейера | %s |", results[0].rec.Version)
	b.p("| Tesseract | %s, `-l rus+eng --psm 6`, скан, который не сошёлся, ещё раз с `--psm 4` |", tess)
	b.p("| Железо | %s, %d потоков; %s |", cpu(), runtime.NumCPU(), hw)
	b.p("")
	b.formats(results)
	b.fields(results)
	b.checks(results)
	b.docs(results)
	return b.String()
}

func (b *md) formats(results []result) {
	b.p("## По форматам")
	b.p("")
	b.p("a и b это текстовые PDF двух банков, c скан PNG (один завёрнут в PDF),")
	b.p("ловушка это выписка с приходом, завышенным в 1,5 раза.")
	b.p("")
	b.p("| Формат | Документов | Поля верно | Документ верен целиком | Цель по полям | |")
	b.p("|---|---|---|---|---|---|")
	for _, f := range []string{"a", "b", "c", "ловушка"} {
		docs, ok, total, whole := 0, 0, 0, 0
		for i := range results {
			r := &results[i]
			if r.format != f {
				continue
			}
			docs++
			ok += r.correct()
			total += len(r.fields)
			if r.correct() == len(r.fields) {
				whole++
			}
		}
		verdict := "достигнута"
		if ratio(ok, total) < target[f] {
			verdict = "ниже цели"
		}
		b.p("| %s | %d | %d из %d, %s | %d из %d | %.0f%% | %s |",
			f, docs, ok, total, pct(ratio(ok, total)), whole, docs, target[f]*100, verdict)
	}
	b.p("")
}

func (b *md) fields(results []result) {
	b.p("## По полям")
	b.p("")
	b.p("| Документ | Поле | Верно |")
	b.p("|---|---|---|")
	for _, kind := range []string{"statement", "financials"} {
		var names []string
		ok, total := map[string]int{}, map[string]int{}
		for i := range results {
			if !strings.Contains(results[i].file, "_"+kind+"_") {
				continue
			}
			for _, f := range results[i].fields {
				if total[f.name] == 0 {
					names = append(names, f.name)
				}
				total[f.name]++
				if f.ok {
					ok[f.name]++
				}
			}
		}
		for _, n := range names {
			b.p("| %s | %s | %d из %d, %s |", kind, n, ok[n], total[n], pct(ratio(ok[n], total[n])))
		}
	}
	b.p("")
}

func (b *md) checks(results []result) {
	caught, alarm, missed, down := 0, 0, 0, 0
	took := make([]time.Duration, 0, len(results))
	var sum time.Duration
	for i := range results {
		r := &results[i]
		wrong := r.correct() < len(r.fields)
		switch {
		case r.down:
			down++
		case r.failed && wrong:
			caught++
		case r.failed:
			alarm++
		case wrong:
			missed++
		}
		took = append(took, r.rec.Took)
		sum += r.rec.Took
	}
	slices.Sort(took)

	b.p("## Проверки в Go")
	b.p("")
	b.p("Документ с ошибкой модели или OCR лучше получить `failed` и ввод руками, чем неверные цифры в форме.")
	b.p("")
	b.p("- ошибку поймала проверка, документ `failed`: %d;", caught)
	b.p("- проверка не пропустила верно распознанный документ: %d;", alarm)
	b.p("- ошибка прошла проверку и попала бы в форму на подтверждение клиенту: %d;", missed)
	b.p("- модель не уложилась в таймаут, документ `failed` без полей: %d.", down)
	b.p("")
	b.p("## Время")
	b.p("")
	b.p("На документ p50 %s, p95 %s, весь прогон %s. Первый документ дольше: модель грузится в память.",
		quantile(took, 0.5), quantile(took, 0.95), sum.Round(time.Second))
	b.p("На CPU без видеокарты время на документ будет больше, замеряем отдельно. Точность от железа не зависит.")
	b.p("")
}

func (b *md) docs(results []result) {
	b.p("## По документам")
	b.p("")
	b.p("| Файл | Текст | Поля верно | Проверка в Go | Время |")
	b.p("|---|---|---|---|---|")
	for i := range results {
		r := &results[i]
		check := "прошла"
		switch {
		case r.down:
			check = "failed: не уложилась в OLLAMA_TIMEOUT"
		case r.failed:
			check = "failed: " + problems(r.rec.Problems)
		}
		var wrong []string
		for _, f := range r.fields {
			if !f.ok {
				wrong = append(wrong, f.name)
			}
		}
		fields := fmt.Sprintf("%d из %d", r.correct(), len(r.fields))
		if len(wrong) > 0 {
			fields += ", мимо: " + strings.Join(wrong, ", ")
		}
		b.p("| %s | %s | %s | %s | %s |", r.file, cmp.Or(string(r.rec.Source), "-"), fields, check,
			r.rec.Took.Round(100*time.Millisecond))
	}
}

func problems(errs []domain.FieldError) string {
	if len(errs) == 0 {
		return "ответ не по схеме или файл не читается"
	}
	var s []string
	for _, e := range errs {
		s = append(s, e.Field)
	}
	return strings.Join(slices.Compact(s), ", ")
}

func ratio(ok, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(ok) / float64(total)
}

func pct(v float64) string { return fmt.Sprintf("%.1f%%", v*100) }

func quantile(sorted []time.Duration, q float64) time.Duration {
	return sorted[int(q*float64(len(sorted)-1)+0.5)].Round(100 * time.Millisecond)
}

// cpu: на Linux из /proc/cpuinfo, на остальных EVAL_CPU, имя процессора без cgo иначе не достать
func cpu() string {
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for l := range strings.SplitSeq(string(b), "\n") {
			if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "model name" {
				return strings.TrimSpace(v)
			}
		}
	}
	return cmp.Or(os.Getenv("EVAL_CPU"), runtime.GOARCH)
}

func tesseractVersion(bin string) string {
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "?"
	}
	first, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(first)
}

// offload спрашивает у Ollama, сколько модели легло в видеопамять: от этого время отличается в разы
func offload(base, model string) string {
	resp, err := http.Get(base + "/api/ps")
	if err != nil {
		return "Ollama не ответила на /api/ps"
	}
	defer resp.Body.Close()
	var ps struct {
		Models []struct {
			Name     string `json:"name"`
			SizeVRAM int64  `json:"size_vram"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&ps) != nil {
		return "Ollama не ответила на /api/ps"
	}
	for _, m := range ps.Models {
		if m.Name == model && m.SizeVRAM > 0 {
			return fmt.Sprintf("модель в видеопамяти %s, %d МБ", gpu(), m.SizeVRAM>>20)
		}
	}
	return "только CPU, без GPU"
}

// gpu: имя карты от nvidia-smi, другой видеопамяти у Ollama на наших машинах нет
func gpu() string {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader").Output()
	if err != nil {
		return "(nvidia-smi не ответил)"
	}
	return strings.TrimSpace(string(out))
}
