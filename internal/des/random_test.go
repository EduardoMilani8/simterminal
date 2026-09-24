package des

import (
	"math"
	"testing"
)

func TestStreamsAreReproducibleAndIndependent(t *testing.T) {
	a1, a2 := NewStream(5, "chegadas"), NewStream(5, "chegadas")
	b := NewStream(5, "atributos")
	other := NewStream(6, "chegadas")

	same, diffName, diffSeed := true, false, false
	for i := 0; i < 100; i++ {
		x, y := a1.Float64(), a2.Float64()
		if x != y {
			same = false
		}
		if x != b.Float64() {
			diffName = true
		}
		if x != other.Float64() {
			diffSeed = true
		}
	}
	if !same {
		t.Error("mesmo nome e semente deveriam gerar a mesma sequência")
	}
	if !diffName || !diffSeed {
		t.Error("nomes ou sementes diferentes deveriam gerar sequências diferentes")
	}
}

func TestTriangularMean(t *testing.T) {
	s := NewStream(1, "t")
	const n = 200_000
	sum := 0.0
	for i := 0; i < n; i++ {
		v := s.Triangular(35, 48, 95)
		if v < 35 || v > 95 {
			t.Fatalf("valor %v fora de [35, 95]", v)
		}
		sum += v
	}
	want := (35.0 + 48 + 95) / 3
	if got := sum / n; math.Abs(got-want) > 0.2 {
		t.Errorf("média triangular = %.2f, esperava %.2f", got, want)
	}
}

// TestNextArrivalFollowsProfile: com thinning, a contagem média de
// chegadas em cada hora tem que bater com a taxa daquela hora.
func TestNextArrivalFollowsProfile(t *testing.T) {
	profile := []float64{4, 30, 10, 0, 20} // chegadas por hora
	rate := func(t float64) float64 {
		h := int(t / 60)
		if h < 0 || h >= len(profile) {
			return 0
		}
		return profile[h] / 60
	}
	maxRate := 30.0 / 60
	until := 60.0 * float64(len(profile))

	const reps = 4000
	counts := make([]float64, len(profile))
	s := NewStream(3, "chegadas")
	for r := 0; r < reps; r++ {
		now := 0.0
		for {
			next, ok := s.NextArrival(now, rate, maxRate, until)
			if !ok {
				break
			}
			counts[int(next/60)]++
			now = next
		}
	}
	for h, want := range profile {
		got := counts[h] / reps
		// desvio padrão da média de Poisson: sqrt(λ/reps)
		tol := 4*math.Sqrt(want/reps) + 1e-9
		if math.Abs(got-want) > tol {
			t.Errorf("hora %d: %.2f chegadas em média, esperava %.0f (±%.2f)", h, got, want, tol)
		}
	}
}
