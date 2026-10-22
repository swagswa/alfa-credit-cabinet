package docgen

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/rand/v2"
	"time"

	"codeberg.org/go-pdf/fpdf"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

var (
	//go:embed fonts/PTSans-Regular.ttf
	regularTTF []byte
	//go:embed fonts/PTSans-Bold.ttf
	boldTTF []byte
)

func newPDF() *fpdf.Fpdf {
	f := fpdf.New("P", "mm", "A4", "")
	// дата и порядок объектов фиксированы, иначе файл другой при каждом запуске
	stamp := time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)
	f.SetCreationDate(stamp)
	f.SetModificationDate(stamp)
	f.SetCatalogSort(true)
	f.SetAutoPageBreak(false, 0)
	return f
}

func output(f *fpdf.Fpdf) ([]byte, error) {
	var b bytes.Buffer
	if err := f.Output(&b); err != nil {
		return nil, fmt.Errorf("pdf: %w", err)
	}
	return b.Bytes(), nil
}

func renderPDF(p page) ([]byte, error) {
	f := newPDF()
	// UTF-8 шрифт fpdf пишет с ToUnicode, без этого кириллицу из PDF не достать
	f.AddUTF8FontFromBytes("pt", "", regularTTF)
	f.AddUTF8FontFromBytes("pt", "B", boldTTF)
	f.AddPage()
	f.SetLineWidth(0.3)
	for _, y := range p.rules {
		f.Line(left, y, right, y)
	}
	for _, t := range p.texts {
		style := ""
		if t.bold {
			style = "B"
		}
		f.SetFont("pt", style, t.size)
		x := t.x
		if t.right {
			x -= f.GetStringWidth(t.s)
		}
		f.Text(x, t.y, t.s)
	}
	return output(f)
}

const dpi = 150

func px(mm float64) int { return int(math.Round(mm / 25.4 * dpi)) }

// scan рисует страницу в PNG 150 dpi в оттенках серого, поворачивает на 1-3°,
// чуть размывает и шумит. Шум и угол от seed, файл воспроизводится.
func scan(p page, seed string, asPDF bool) ([]byte, error) {
	src, err := rasterize(p)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(seed))
	rng := rand.New(rand.NewPCG(binary.BigEndian.Uint64(h[:8]), binary.BigEndian.Uint64(h[8:16])))

	b := src.Bounds()
	img := image.NewGray(b)
	draw.Draw(img, b, image.White, image.Point{}, draw.Src)
	a := (1 + 2*rng.Float64()) * math.Pi / 180
	if rng.IntN(2) == 0 {
		a = -a
	}
	sin, cos := math.Sincos(a)
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	rot := f64.Aff3{cos, -sin, cx - cos*cx + sin*cy, sin, cos, cy - sin*cx - cos*cy}
	xdraw.BiLinear.Transform(img, rot, src, b, xdraw.Over, nil)

	// размытие: уменьшить и растянуть обратно
	small := image.NewGray(image.Rect(0, 0, b.Dx()*3/4, b.Dy()*3/4))
	xdraw.BiLinear.Scale(small, small.Bounds(), img, b, xdraw.Src, nil)
	xdraw.BiLinear.Scale(img, b, small, small.Bounds(), xdraw.Src, nil)

	// 16 оттенков серого: PNG в 4 бита весит в разы меньше, а OCR разницы не видит
	gray16 := make(color.Palette, 16)
	for i := range gray16 {
		gray16[i] = color.Gray{Y: uint8(i * 17)}
	}
	out := image.NewPaletted(b, gray16)
	for i, v := range img.Pix {
		out.Pix[i] = uint8((math.Max(0, math.Min(255, float64(v)+rng.NormFloat64()*5)) + 8) / 17)
	}

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("png: %w", err)
	}
	if !asPDF {
		return buf.Bytes(), nil
	}
	f := newPDF()
	f.AddPage()
	opt := fpdf.ImageOptions{ImageType: "PNG"}
	f.RegisterImageOptionsReader("scan", opt, &buf)
	f.ImageOptions("scan", 0, 0, 210, 297, false, opt, 0, "")
	return output(f)
}

func rasterize(p page) (*image.Gray, error) {
	img := image.NewGray(image.Rect(0, 0, px(210), px(297)))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	ink := image.NewUniform(color.Gray{Y: 35})
	for _, y := range p.rules {
		draw.Draw(img, image.Rect(px(left), px(y)-1, px(right), px(y)+1), ink, image.Point{}, draw.Src)
	}

	fonts := map[bool]*opentype.Font{}
	for bold, ttf := range map[bool][]byte{false: regularTTF, true: boldTTF} {
		f, err := opentype.Parse(ttf)
		if err != nil {
			return nil, fmt.Errorf("шрифт: %w", err)
		}
		fonts[bold] = f
	}
	type key struct {
		bold bool
		size float64
	}
	faces := map[key]font.Face{}
	for _, t := range p.texts {
		k := key{t.bold, t.size}
		if faces[k] == nil {
			face, err := opentype.NewFace(
				fonts[t.bold],
				&opentype.FaceOptions{Size: t.size, DPI: dpi, Hinting: font.HintingFull},
			)
			if err != nil {
				return nil, fmt.Errorf("шрифт: %w", err)
			}
			faces[k] = face
		}
		d := font.Drawer{Dst: img, Src: ink, Face: faces[k]}
		x := fixed.I(px(t.x))
		if t.right {
			x -= d.MeasureString(t.s)
		}
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(px(t.y))}
		d.DrawString(t.s)
	}
	return img, nil
}
