package stats

import (
	"math"
	"sort"
)

// Series acumula observações (tempos de espera, tempos de ciclo) e
// resume o que aconteceu na simulação.
type Series struct {
	values []float64
}

// Add registra uma observação.
func (s *Series) Add(v float64) { s.values = append(s.values, v) }

// N devolve quantas observações foram registradas.
func (s *Series) N() int { return len(s.values) }

// Mean devolve a média das observações.
func (s *Series) Mean() float64 {
	if len(s.values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range s.values {
		sum += v
	}
	return sum / float64(len(s.values))
}

// StdDev devolve o desvio padrão amostral.
func (s *Series) StdDev() float64 {
	n := len(s.values)
	if n < 2 {
		return 0
	}
	m := s.Mean()
	sum := 0.0
	for _, v := range s.values {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(n-1))
}

// Max devolve a maior observação.
func (s *Series) Max() float64 {
	if len(s.values) == 0 {
		return 0
	}
	max := s.values[0]
	for _, v := range s.values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

// Percentile devolve o percentil p (0..100). O p95 costuma dizer mais
// sobre a dor do motorista do que a média.
func (s *Series) Percentile(p float64) float64 {
	if len(s.values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), s.values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
