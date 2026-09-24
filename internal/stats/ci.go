package stats

import "math"

// Interval é uma média com seu intervalo de confiança de 95%:
// Mean ± HalfWidth.
type Interval struct {
	Mean      float64
	HalfWidth float64
	N         int
}

// Lo devolve o limite inferior do intervalo.
func (iv Interval) Lo() float64 { return iv.Mean - iv.HalfWidth }

// Hi devolve o limite superior do intervalo.
func (iv Interval) Hi() float64 { return iv.Mean + iv.HalfWidth }

// MeanCI devolve a média de xs com intervalo de confiança de 95% pela t
// de Student. Cada valor deve ser uma réplica independente — observações
// de dentro de uma mesma rodada são correlacionadas e dariam um
// intervalo falsamente estreito.
func MeanCI(xs []float64) Interval {
	var s Series
	for _, x := range xs {
		s.Add(x)
	}
	n := s.N()
	iv := Interval{Mean: s.Mean(), N: n}
	if n >= 2 {
		iv.HalfWidth = TQuantile975(n-1) * s.StdDev() / math.Sqrt(float64(n))
	}
	return iv
}

// PairedDiffCI devolve o intervalo de confiança de 95% de a[i] − b[i].
// Com common random numbers (as mesmas sementes nos dois cenários) o
// ruído do sorteio se cancela na diferença, e o intervalo encolhe muito.
func PairedDiffCI(a, b []float64) Interval {
	n := min(len(a), len(b))
	d := make([]float64, n)
	for i := range d {
		d[i] = a[i] - b[i]
	}
	return MeanCI(d)
}

// UnpairedDiffCI devolve o intervalo de confiança de 95% da diferença de
// médias tratando as amostras como independentes (Welch). Serve de
// comparação para mostrar o quanto o pareamento ganha.
func UnpairedDiffCI(a, b []float64) Interval {
	var sa, sb Series
	for _, x := range a {
		sa.Add(x)
	}
	for _, x := range b {
		sb.Add(x)
	}
	na, nb := float64(sa.N()), float64(sb.N())
	va, vb := sa.StdDev()*sa.StdDev()/na, sb.StdDev()*sb.StdDev()/nb
	iv := Interval{Mean: sa.Mean() - sb.Mean(), N: min(sa.N(), sb.N())}
	if va+vb == 0 || na < 2 || nb < 2 {
		return iv
	}
	// graus de liberdade de Welch–Satterthwaite
	df := (va + vb) * (va + vb) / (va*va/(na-1) + vb*vb/(nb-1))
	iv.HalfWidth = TQuantile975(int(df)) * math.Sqrt(va+vb)
	return iv
}

var t975 = [...]float64{
	12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
	2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
	2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042,
}

// TQuantile975 devolve o quantil 97,5% da t de Student com df graus de
// liberdade — o multiplicador do intervalo de confiança bilateral de 95%.
func TQuantile975(df int) float64 {
	if df < 1 {
		return math.Inf(1)
	}
	if df <= len(t975) {
		return t975[df-1]
	}
	// expansão de Cornish-Fisher em torno da normal; erro < 0,001 para
	// df > 30
	const z = 1.959963984540054
	n := float64(df)
	z3, z5 := z*z*z, z*z*z*z*z
	return z + (z3+z)/(4*n) + (5*z5+16*z3+3*z)/(96*n*n)
}
