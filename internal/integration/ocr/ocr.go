// Package ocr достаёт текст из файла документа: текстовый слой PDF как есть,
// сканы через Tesseract CLI. CLI, а не gosseract: тот тянет cgo, а сборка у нас
// с CGO_ENABLED=0.
package ocr

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

const (
	// меньше символов на страницу считаем сканом: в такой PDF текст только картинкой
	minPageText = 200
	maxPages    = 5
)

var columns = regexp.MustCompile(` {2,}`)

type Client struct {
	bin     string
	timeout time.Duration
}

// New принимает путь к tesseract и таймаут на одну страницу. Каталог языков
// tesseract сам берёт из TESSDATA_PREFIX, переменная доходит до него через окружение.
func New(bin string, timeout time.Duration) *Client {
	return &Client{bin: bin, timeout: timeout}
}

// Text отдаёт текст документа и откуда он взят. Битый файл, лишние страницы
// и пустой результат это domain.ErrInvalid: повтор тут не поможет.
func (c *Client) Text(ctx context.Context, contentType string, file []byte) (string, domain.TextSource, error) {
	if contentType != "application/pdf" {
		s, err := c.image(ctx, file)
		return s, domain.TextOCR, err
	}
	s, err := pdfText(file)
	if err != nil {
		return "", "", err
	}
	if s != "" {
		return s, domain.TextPDF, nil
	}
	pages, err := pdfImages(ctx, file)
	if err != nil {
		return "", "", err
	}
	var b strings.Builder
	for _, p := range pages {
		s, err := c.image(ctx, p)
		if err != nil {
			return "", "", err
		}
		b.WriteString(s)
		b.WriteString("\n")
	}
	return b.String(), domain.TextOCR, nil
}

func (c *Client) image(ctx context.Context, img []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// psm 6: страница одним блоком, на таблицах выписки это лучше автоматической
	// разметки. Без preserve_interword_spaces колонки и разряды внутри суммы
	// разделены одинаковым пробелом, и модель режет «3586 208,56» не там.
	args := []string{"stdin", "stdout", "-l", "rus+eng", "--psm", "6", "-c", "preserve_interword_spaces=1"}
	cmd := exec.CommandContext(ctx, c.bin, args...) //nolint:gosec // путь из конфига, аргументы наши
	cmd.Stdin = bytes.NewReader(img)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	// колонки сводим к трём пробелам, как в строках текстового PDF
	s := strings.TrimSpace(columns.ReplaceAllString(string(out), "   "))
	if s == "" {
		return "", fmt.Errorf("%w: на картинке не нашлось текста", domain.ErrInvalid)
	}
	return s, nil
}

// pdfText отдаёт пустую строку, если хоть одна страница похожа на скан
func pdfText(file []byte) (text string, err error) {
	r, err := pdf.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return "", fmt.Errorf("%w: не читается как PDF: %w", domain.ErrInvalid, err)
	}
	n := r.NumPage()
	if n > maxPages {
		return "", fmt.Errorf("%w: в документе %d страниц, больше %d не распознаём", domain.ErrInvalid, n, maxPages)
	}
	// на кривом содержимом страницы библиотека паникует, а файл присылает клиент
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("%w: не читается как PDF: %v", domain.ErrInvalid, p)
		}
	}()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		page := lines(r.Page(i).Content().Text)
		if len([]rune(page)) < minPageText {
			return "", nil
		}
		b.WriteString(page)
	}
	return b.String(), nil
}

// lines собирает буквы в строки страницы. GetPlainText отдаёт каждую ячейку
// таблицы отдельной строкой, и модель путает, к какому месяцу какая сумма.
// Буквы одного куска текста приходят с одной точкой начала, куски одной
// строки с одной базовой линией.
func lines(glyphs []pdf.Text) string {
	type run struct {
		x, y float64
		s    string
	}
	var runs []run
	for _, g := range glyphs {
		if n := len(runs); n > 0 && runs[n-1].x == g.X && runs[n-1].y == g.Y {
			runs[n-1].s += g.S
			continue
		}
		runs = append(runs, run{g.X, g.Y, g.S})
	}
	// y в PDF растёт снизу вверх
	slices.SortStableFunc(runs, func(a, b run) int {
		return cmp.Or(cmp.Compare(b.y, a.y), cmp.Compare(a.x, b.x))
	})
	var b strings.Builder
	for i, r := range runs {
		switch {
		case i == 0:
		case r.y != runs[i-1].y:
			b.WriteByte('\n')
		default:
			b.WriteString("   ")
		}
		b.WriteString(r.s)
	}
	b.WriteByte('\n')
	return b.String()
}

// pdfImages достаёт картинки страниц скана. Рендер PDF не нужен: в скане
// страница и есть картинка.
func pdfImages(ctx context.Context, file []byte) ([][]byte, error) {
	var pages [][]byte
	// stateless: иначе pdfcpu полезет писать конфиг в домашний каталог, а у воркера его нет
	conf := model.NewStatelessConfiguration()
	err := api.ExtractImages(ctx, bytes.NewReader(file), nil, func(img model.Image, _ bool, _ int) error {
		b, err := io.ReadAll(img)
		pages = append(pages, b)
		return err //nolint:wrapcheck // pdfcpu сам подпишет страницу
	}, conf)
	if err != nil {
		return nil, fmt.Errorf("%w: картинки из PDF: %w", domain.ErrInvalid, err)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("%w: в PDF нет ни текста, ни картинок", domain.ErrInvalid)
	}
	return pages, nil
}
