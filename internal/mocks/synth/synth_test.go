package synth

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	// диапазоны это бины F2 и F3 из rules/scoring.yaml, в которые должна попасть компания
	for _, tc := range []struct {
		inn              string
		avg              int64 // руб в месяц
		trendLo, trendHi float64
		cvLo, cvHi       float64
	}{
		{"0000100012", 1_200_000, 0.9, 1.1, 0, 0.3},
		{"000020001010", 2_000_000, 0.9, 1.1, 0.3, 0.5},
		{"0000300011", 3_500_000, 1.1, 9, 0.5, 9},
		{"0000400016", 4_000_000, 0, 0.7, 0.5, 9},
	} {
		t.Run(tc.inn, func(t *testing.T) {
			c, err := New(tc.inn)
			require.NoError(t, err)
			require.Len(t, c.Months, 12)
			require.Equal(t, "2025-10", c.Months[0].Start.Format("2006-01"))
			require.Equal(t, "2026-09", c.Months[11].Start.Format("2006-01"))
			require.InDelta(t, tc.avg*12*100, c.Income(), 12*100, "округление до рубля в каждом месяце")

			var in [12]float64
			for i, m := range c.Months {
				require.Zero(t, m.In%100, "поступления в целых рублях")
				require.Positive(t, m.Out)
				in[i] = float64(m.In)
			}
			trend := sum(in[6:]) / sum(in[:6])
			require.True(t, trend >= tc.trendLo && trend < tc.trendHi, "тренд %.3f", trend)
			cv := stddev(in[:]) / (sum(in[:]) / 12)
			require.True(t, cv >= tc.cvLo && cv < tc.cvHi, "вариация %.3f", cv)

			require.Positive(t, c.Closing())
			require.GreaterOrEqual(t, c.BalanceTotal, c.LiabilitiesLong+c.LiabilitiesShort)
			require.LessOrEqual(t, abs(c.NetProfit), c.Income())

			again, err := New(tc.inn)
			require.NoError(t, err)
			require.Equal(t, c, again)
		})
	}
}

func TestNewGenerated(t *testing.T) {
	for _, inn := range []string{"0012345673", "001234567887"} {
		t.Run(inn, func(t *testing.T) {
			c, err := New(inn)
			require.NoError(t, err)
			require.NotEmpty(t, c.Name)
			require.Equal(t, len(inn) == 12, c.IsIP())
			require.Positive(t, c.Income())
			require.Positive(t, c.Closing())
		})
	}
}

func TestNewInvalid(t *testing.T) {
	for _, inn := range []string{"", "123", "0000100013", "00001000120"} {
		_, err := New(inn)
		require.Error(t, err, inn)
	}
}

func sum(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v
	}
	return s
}

func stddev(x []float64) float64 {
	m := sum(x) / float64(len(x))
	var s float64
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(x)))
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
