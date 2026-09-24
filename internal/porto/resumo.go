package porto

import (
	"math"
	"sort"

	"github.com/eduardomilani8/simterminal/internal/stats"
)

// Resumo são os números de um grupo num cenário, contando só os navios
// que chegaram dentro do período.
type Resumo struct {
	Navios        int
	EsperaMedia   float64 // horas, da chegada à atracação
	EsperaMediana float64
	EsperaP90     float64
	NavioDias     float64 // soma das esperas, em dias
	Ocupacao      float64 // fração do tempo com as posições ocupadas
}

// Resumo resume o replay.
func (r Resultado) Resumo() Resumo {
	var esperas []float64
	for i := range r.Dados.Navios {
		if r.Dados.NoPeriodo(&r.Dados.Navios[i]) {
			esperas = append(esperas, r.Atracou[i]-r.Chegada[i])
		}
	}
	return resumir(esperas, ocupacao(r.Atracou, r.Saiu, r.Posicoes, r.Janela))
}

// ResumoReal resume o que aconteceu de fato, direto do dado.
func ResumoReal(d *DadosGrupo) Resumo {
	var esperas []float64
	a := make([]float64, len(d.Navios))
	s := make([]float64, len(d.Navios))
	pos := 0
	for _, k := range d.Posicoes {
		pos += k
	}
	for i, n := range d.Navios {
		a[i], s[i] = n.Atracacao, n.Desatracacao
		if d.NoPeriodo(&n) {
			esperas = append(esperas, n.Atracacao-n.Chegada)
		}
	}
	return resumir(esperas, ocupacao(a, s, pos, d.Fim))
}

func resumir(esperas []float64, ocup float64) Resumo {
	var s stats.Series
	for _, x := range esperas {
		s.Add(x)
	}
	return Resumo{
		Navios:        s.N(),
		EsperaMedia:   s.Mean(),
		EsperaMediana: medianaDe(esperas),
		EsperaP90:     s.Percentile(90),
		NavioDias:     s.Mean() * float64(s.N()) / 24,
		Ocupacao:      ocup,
	}
}

// medianaDe usa a média dos dois do meio quando n é par.
func medianaDe(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	ys := append([]float64(nil), xs...)
	sort.Float64s(ys)
	m := len(ys) / 2
	if len(ys)%2 == 1 {
		return ys[m]
	}
	return (ys[m-1] + ys[m]) / 2
}

// ocupacao é a área sob a curva de navios no berço dentro de [0, janela],
// dividida pelas posições e pela janela.
func ocupacao(atracou, saiu []float64, posicoes int, janela float64) float64 {
	if posicoes == 0 || janela <= 0 {
		return 0
	}
	h := 0.0
	for i := range atracou {
		ini, fim := math.Max(atracou[i], 0), math.Min(saiu[i], janela)
		if fim > ini {
			h += fim - ini
		}
	}
	return h / (float64(posicoes) * janela)
}
