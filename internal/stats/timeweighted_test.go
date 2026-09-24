package stats

import (
	"math"
	"testing"
)

func TestTimeWeightedMean(t *testing.T) {
	var w TimeWeighted
	w.Set(0, 1)  // 1 de 0 a 10
	w.Set(10, 3) // 3 de 10 a 20
	w.Set(20, 0) // 0 de 20 a 40

	if got, want := w.Mean(40), (10.0+30)/40; math.Abs(got-want) > 1e-12 {
		t.Errorf("média = %v, esperava %v", got, want)
	}
	if w.Max() != 3 {
		t.Errorf("máximo = %v, esperava 3", w.Max())
	}
}

func TestTimeWeightedReset(t *testing.T) {
	var w TimeWeighted
	w.Set(0, 10)
	w.Set(5, 2)
	w.Reset(8) // descarta o pico de 10; segue valendo 2
	w.Set(12, 4)

	// 2 de 8 a 12, 4 de 12 a 16
	if got, want := w.Mean(16), (8.0+16)/8; math.Abs(got-want) > 1e-12 {
		t.Errorf("média após reset = %v, esperava %v", got, want)
	}
	if w.Max() != 4 {
		t.Errorf("máximo após reset = %v, esperava 4", w.Max())
	}
}
