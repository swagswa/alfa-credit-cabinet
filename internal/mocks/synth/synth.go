// Package synth считает синтетические финансы компании по ИНН. Это один источник
// для моков (ФНС, обороты в банке) и для генератора документов, поэтому выписка
// по любому ИНН сходится с ответом мока ФНС.
package synth

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/swagswa/alfa-credit-cabinet/internal/domain"
)

// Version входит в seed: поменяли генератор, подняли версию, старые заявки
// по ней понятно как пересчитать.
const Version = "1"

type Month struct {
	Start time.Time
	In    int64 // поступления, коп
	Out   int64 // списания, коп
}

type Company struct {
	INN     string
	Name    string
	Opening int64   // остаток на начало периода, коп
	Months  []Month // 12 полных месяцев до октября 2026, от старого к новому

	// отчётность за Year, коп
	Year             int
	NetProfit        int64
	LiabilitiesLong  int64
	LiabilitiesShort int64
	BalanceTotal     int64
}

// Income это выручка за прошлый год в моке ФНС. Упрощение: равна поступлениям
// за 12 месяцев выписки, иначе у IT-компании с трендом 1,4 честная выписка
// разошлась бы с налоговой больше чем на 20%.
func (c Company) Income() int64 {
	in, _ := c.Turnover()
	return in
}

func (c Company) Turnover() (in, out int64) {
	for _, m := range c.Months {
		in, out = in+m.In, out+m.Out
	}
	return in, out
}

func (c Company) Closing() int64 {
	in, out := c.Turnover()
	return c.Opening + in - out
}

func (c Company) IsIP() bool { return len(c.INN) == 12 }

// profile задаёт то, что видно в скоринге: средняя выручка в месяц (руб),
// тренд последних 6 мес к предыдущим 6 и коэффициент вариации оборотов.
type profile struct {
	name  string
	avg   float64
	trend float64
	cv    float64
}

// demo это компании из 06 раздел 6.5, остальные цифры у них тоже из seed.
func demo(inn string) (profile, bool) {
	p, ok := map[string]profile{
		"0000100012":   {"ООО «Кофейня на Садовой»", 1_200_000, 1.05, 0.25},
		"000020001010": {"ИП Логинов Сергей Петрович", 2_000_000, 0.95, 0.35},
		"0000300011":   {"ООО «Код и Ко»", 3_500_000, 1.4, 0.55},
		"0000400016":   {"ООО «СтройМонолит»", 4_000_000, 0.6, 0.6},
		"0000500010":   {"ООО «Тихая гавань»", 900_000, 1.0, 0.2},
	}[inn]
	return p, ok
}

func New(inn string) (Company, error) {
	if !domain.ValidINN(inn) {
		return Company{}, fmt.Errorf("инн %q: неверная длина или контрольные цифры", inn)
	}
	rng := RNG(inn + Version)
	q := rng.Float64() // скрытое качество компании, от него коррелированно всё остальное

	p, ok := demo(inn)
	if !ok {
		p = profile{
			name:  name(rng, len(inn) == 12),
			avg:   math.Round(400_000 * math.Exp2(4*q+rng.NormFloat64()/2)),
			trend: clamp(0.7+0.6*q+rng.NormFloat64()/10, 0.5, 1.6),
			cv:    clamp(0.6-0.4*q+rng.NormFloat64()/12, 0.08, 0.75),
		}
	}

	c := Company{INN: inn, Name: p.name, Year: 2025, Opening: kop(p.avg * (0.1 + 0.4*rng.Float64()))}
	start := time.Date(2025, time.October, 1, 0, 0, 0, 0, time.UTC)
	for i, v := range series(rng, p) {
		in := rub(p.avg * v)
		// списываем 93-100% поступившего, остаток на счёте не уходит в минус
		out := kop(float64(in) / 100 * (0.93 + 0.07*rng.Float64()))
		c.Months = append(c.Months, Month{start.AddDate(0, i, 0), in, out})
	}

	revenue := float64(c.Income() / 100)
	c.NetProfit = rub(revenue * (0.15*q - 0.03))
	c.LiabilitiesShort = rub(revenue * (0.08 + 0.1*rng.Float64()))
	c.LiabilitiesLong = rub(revenue * 0.3 * (1 - q) * rng.Float64())
	c.BalanceTotal = rub(float64(c.LiabilitiesShort+c.LiabilitiesLong) / 100 / (0.4 + 0.4*q))
	return c, nil
}

// RNG даёт генератор от строки: тот же seed, те же числа при каждом запуске
func RNG(seed string) *rand.Rand {
	h := sha256.Sum256([]byte(seed))
	return rand.New(rand.NewPCG(binary.BigEndian.Uint64(h[:8]), binary.BigEndian.Uint64(h[8:16])))
}

// series даёт 12 множителей к средней выручке: среднее 1, отношение второго
// полугодия к первому ровно trend, разброс около cv. Уровни полугодий сами
// дают часть вариации, шум добирает остальное.
func series(rng *rand.Rand, p profile) []float64 {
	const half = 6
	lv := [2]float64{2 / (1 + p.trend), 2 * p.trend / (1 + p.trend)}
	d := (lv[1] - lv[0]) / 2
	s := math.Sqrt(math.Max(0, (p.cv*p.cv-d*d)*2/(lv[0]*lv[0]+lv[1]*lv[1])))

	out := make([]float64, 0, 2*half)
	for _, l := range lv {
		z := make([]float64, half)
		var mean, sq float64
		for i := range z {
			z[i] = rng.NormFloat64()
			mean += z[i] / half
		}
		for i := range z {
			z[i] -= mean
			sq += z[i] * z[i] / half
		}
		var sum float64
		for i := range z {
			z[i] = math.Max(0.15, 1+s*z[i]/math.Sqrt(sq))
			sum += z[i]
		}
		// после отсечки снизу перенормируем, чтобы тренд остался точным
		for i := range z {
			out = append(out, l*z[i]*half/sum)
		}
	}
	return out
}

func name(rng *rand.Rand, ip bool) string {
	if ip {
		last := []string{"Карпов", "Белов", "Зуев", "Орлов", "Титов", "Гусев"}
		first := []string{"Андрей", "Илья", "Олег", "Павел", "Роман"}
		mid := []string{"Ильич", "Олегович", "Андреевич", "Павлович"}
		return "ИП " + last[rng.IntN(len(last))] + " " + first[rng.IntN(len(first))] + " " + mid[rng.IntN(len(mid))]
	}
	a := []string{"Северный", "Новый", "Первый", "Городской", "Восточный", "Деловой"}
	b := []string{"Двор", "Ряд", "Склад", "Маршрут", "Цех", "Квартал"}
	return "ООО «" + a[rng.IntN(len(a))] + " " + b[rng.IntN(len(b))] + "»"
}

// rub округляет до целых рублей и переводит в копейки
func rub(x float64) int64 { return int64(math.Round(x)) * 100 }

func kop(x float64) int64 { return int64(math.Round(x * 100)) }

func clamp(x, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, x)) }
