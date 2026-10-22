package ocr_test

import (
	"cmp"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
	"github.com/swagswa/alfa-credit-cabinet/internal/integration/ocr"
)

func TestText(t *testing.T) {
	bin, err := exec.LookPath(cmp.Or(os.Getenv("TESSERACT_PATH"), "tesseract"))
	if err == nil {
		// без rus tesseract не падает, а молча читает кириллицу латиницей
		langs, _ := exec.Command(bin, "--list-langs").Output()
		if !strings.Contains(string(langs), "rus") {
			err = errors.New("нет языка rus, проверь TESSDATA_PREFIX")
		}
	}
	tests := []struct {
		file   string
		source domain.TextSource
		want   []string
	}{
		{"0000300011_statement_a.pdf", domain.TextPDF, []string{"ИНН: 0000300011", "Исходящий остаток   2 563 553,15"}},
		{"000020001010_statement_b.pdf", domain.TextPDF, []string{"Поступления:   23999999.00 руб.", "Сентябрь 2026"}},
		{"0000100012_financials_a.pdf", domain.TextPDF, []string{"Выручка   2110   14 399 999"}},
		{"0000400016_statement_c.png", domain.TextOCR, []string{"0000400016", "Исходящий остаток"}},
		{"0000100012_statement_c.pdf", domain.TextOCR, []string{"0000100012", "Входящий остаток"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if tt.source == domain.TextOCR && err != nil {
				t.Skipf("нет tesseract: %v", err)
			}
			file, rerr := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "documents", tt.file))
			require.NoError(t, rerr)
			ct := "application/pdf"
			if filepath.Ext(tt.file) == ".png" {
				ct = "image/png"
			}

			s, src, terr := ocr.New(bin, 30*time.Second).Text(t.Context(), ct, file)
			require.NoError(t, terr)
			require.Equal(t, tt.source, src)
			for _, w := range tt.want {
				require.Contains(t, s, w)
			}
		})
	}
}

func TestTextInvalid(t *testing.T) {
	_, _, err := ocr.New("tesseract", time.Second).Text(t.Context(), "application/pdf", []byte("не pdf"))
	require.ErrorIs(t, err, domain.ErrInvalid)
}
