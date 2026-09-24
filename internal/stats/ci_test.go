package stats

import (
	"math"
	"math/rand"
	"testing"
)

func TestTQuantile975(t *testing.T) {
	cases := map[int]float64{1: 12.706, 9: 2.262, 29: 2.045, 30: 2.042, 40: 2.021, 60: 2.000, 120: 1.980, 100000: 1.960}
	for df, want := range cases {
		if got := TQuantile975(df); math.Abs(got-want) > 0.002 {
			t.Errorf("t(0,975; %d) = %.4f, esperava %.3f", df, got, want)
		}
	}
}

func TestMeanCI(t *testing.T) {
	iv := MeanCI([]float64{10, 12, 14})
	// média 12, s = 2, t(2) = 4,303 → 4,303 × 2 / √3
	if iv.Mean != 12 || math.Abs(iv.HalfWidth-4.303*2/math.Sqrt(3)) > 1e-9 {
		t.Errorf("IC = %.4f ± %.4f", iv.Mean, iv.HalfWidth)
	}
	if one := MeanCI([]float64{5}); one.HalfWidth != 0 {
		t.Error("uma réplica só não tem intervalo")
	}
}

// TestMeanCICoverage: um IC de 95% tem que conter a média verdadeira em
// ~95% das vezes. Se cobrir bem menos, o multiplicador está errado.
func TestMeanCICoverage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const trials = 4000
	hits := 0
	for i := 0; i < trials; i++ {
		xs := make([]float64, 8)
		for j := range xs {
			xs[j] = rng.NormFloat64()*3 + 50
		}
		if iv := MeanCI(xs); iv.Lo() <= 50 && 50 <= iv.Hi() {
			hits++
		}
	}
	if cov := float64(hits) / trials; math.Abs(cov-0.95) > 0.015 {
		t.Errorf("cobertura = %.3f, esperava ~0.95", cov)
	}
}

func TestPairedDiffCancelsSharedNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	a, b := make([]float64, 30), make([]float64, 30)
	for i := range a {
		shared := rng.NormFloat64() * 20 // o "sorteio" comum às duas rodadas
		a[i] = 100 + shared + rng.NormFloat64()
		b[i] = 90 + shared + rng.NormFloat64()
	}
	p, u := PairedDiffCI(a, b), UnpairedDiffCI(a, b)
	if math.Abs(p.Mean-10) > 1 || p.HalfWidth >= u.HalfWidth/5 {
		t.Errorf("pareado %.1f ± %.2f, não pareado %.1f ± %.2f", p.Mean, p.HalfWidth, u.Mean, u.HalfWidth)
	}
}
