package porto

import (
	"fmt"
	"sort"
)

// Diagnostico descreve um grupo de berços só com o que aconteceu, sem
// simular nada.
type Diagnostico struct {
	Grupo    string
	Bercos   int
	Posicoes int
	Real     Resumo

	// BercoCheio é a fração do tempo de espera dos navios em que todas as
	// posições do grupo estavam ocupadas. Perto de 1: a espera é fila por
	// berço. Baixa: os navios esperavam com berço livre — o motivo está
	// fora do cais (carga, documentos, maré, janela de navegação).
	BercoCheio float64
	// Ultrapassados é a fração dos navios que viu alguém chegar depois e
	// atracar antes: mede o quanto a fila foge da ordem de chegada.
	Ultrapassados float64
	NoBercoMedio  float64 // horas por navio
	// Ocioso é a fração do tempo no berço em que o navio não operava
	// (atracado esperando começar ou esperando sair).
	Ocioso        float64
	ManobraTipica float64 // horas entre um navio sair e o próximo atracar, com fila
	Cargas        []Parcela
}

// Parcela é a fatia de uma mercadoria nas atracações do grupo.
type Parcela struct {
	Codigo string
	Fracao float64
}

// Leitura resume o diagnóstico numa frase: o que parece ser o problema.
func (d Diagnostico) Leitura() string {
	switch {
	case d.Real.Navios == 0:
		return "nenhum navio no período"
	case d.Real.EsperaMediana < 24:
		return "espera baixa: mediana abaixo de um dia"
	case d.BercoCheio >= 0.7 && d.Real.Ocupacao >= 0.8:
		return fmt.Sprintf("congestionamento: em %.0f%% do tempo de espera todos os berços estavam ocupados; mais capacidade ou operação mais rápida reduz a fila", d.BercoCheio*100)
	case d.BercoCheio < 0.4:
		return fmt.Sprintf("não é falta de berço: em %.0f%% do tempo de espera havia berço livre; a causa está fora do cais (carga, documentação, maré) e mais berço não resolve", (1-d.BercoCheio)*100)
	default:
		return fmt.Sprintf("misto: berços cheios em %.0f%% do tempo de espera; parte da espera é fila, parte é outra coisa", d.BercoCheio*100)
	}
}

// Diagnosticar descreve o grupo.
func Diagnosticar(d *DadosGrupo) Diagnostico {
	dg := Diagnostico{
		Grupo:         d.Grupo.Nome,
		Bercos:        len(d.Grupo.Bercos),
		Real:          ResumoReal(d),
		ManobraTipica: d.ManobraTipica[Otimista],
	}
	for _, k := range d.Posicoes {
		dg.Posicoes += k
	}

	cheio, espera := 0.0, 0.0
	ultrapassados, n := 0, 0
	noBerco, ocioso := 0.0, 0.0
	cargas := map[string]int{}
	curva := curvaOcupacao(d)
	for i := range d.Navios {
		nv := &d.Navios[i]
		if !d.NoPeriodo(nv) {
			continue
		}
		n++
		espera += nv.Atracacao - nv.Chegada
		cheio += curva.tempoCheio(nv.Chegada, nv.Atracacao, dg.Posicoes)
		for j := range i { // atracaram antes de i
			if d.Navios[j].Chegada > nv.Chegada {
				ultrapassados++
				break
			}
		}
		noBerco += nv.NoBerco()
		ocioso += nv.T2 + nv.T4
		if nv.Carga != "" {
			cargas[nv.Carga]++
		}
	}
	if n == 0 {
		return dg
	}
	if espera > 0 {
		dg.BercoCheio = cheio / espera
	}
	dg.Ultrapassados = float64(ultrapassados) / float64(n)
	dg.NoBercoMedio = noBerco / float64(n)
	if noBerco > 0 {
		dg.Ocioso = ocioso / noBerco
	}
	for c, k := range cargas {
		dg.Cargas = append(dg.Cargas, Parcela{c, float64(k) / float64(n)})
	}
	sort.Slice(dg.Cargas, func(i, j int) bool {
		if dg.Cargas[i].Fracao == dg.Cargas[j].Fracao {
			return dg.Cargas[i].Codigo < dg.Cargas[j].Codigo
		}
		return dg.Cargas[i].Fracao > dg.Cargas[j].Fracao
	})
	return dg
}

// curva é o número de navios no berço ao longo do tempo, como degraus.
type curva struct {
	t []float64 // instantes de mudança, crescentes
	v []int     // valor a partir de t[i]
}

func curvaOcupacao(d *DadosGrupo) curva {
	type ev struct {
		t     float64
		delta int
	}
	var evs []ev
	for _, n := range d.Navios {
		evs = append(evs, ev{n.Atracacao, 1}, ev{n.Desatracacao, -1})
	}
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].t == evs[j].t {
			return evs[i].delta < evs[j].delta
		}
		return evs[i].t < evs[j].t
	})
	var c curva
	cur := 0
	for _, e := range evs {
		cur += e.delta
		if k := len(c.t); k > 0 && c.t[k-1] == e.t {
			c.v[k-1] = cur
			continue
		}
		c.t = append(c.t, e.t)
		c.v = append(c.v, cur)
	}
	return c
}

// tempoCheio devolve quantas horas de [a, b] a curva passou em cap ou
// mais.
func (c curva) tempoCheio(a, b float64, cap int) float64 {
	if b <= a {
		return 0
	}
	i := sort.SearchFloat64s(c.t, a) // primeiro t >= a
	cur := 0
	if i > 0 {
		cur = c.v[i-1]
	}
	if i < len(c.t) && c.t[i] == a {
		cur = c.v[i]
		i++
	}
	total, desde := 0.0, a
	for ; i < len(c.t) && c.t[i] < b; i++ {
		if cur >= cap {
			total += c.t[i] - desde
		}
		desde, cur = c.t[i], c.v[i]
	}
	if cur >= cap {
		total += b - desde
	}
	return total
}
