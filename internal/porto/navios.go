package porto

import (
	"container/heap"
	"math"
	"sort"
	"time"

	"github.com/eduardomilani8/simterminal/internal/antaq"
)

// Prontidao diz quando consideramos que o navio podia atracar.
//
// O dado mostra quando o navio chegou e quando atracou, mas não quando
// ficou pronto: 62% dos navios do corredor de grãos de Paranaguá são
// ultrapassados por quem chegou depois, sinal de que muitos chegam antes
// de a carga estar disponível. Como o dado não separa "esperando berço"
// de "esperando carga", todo ganho de cenário sai como uma faixa entre
// as duas leituras.
type Prontidao int

const (
	// Otimista: o navio estava pronto ao chegar. Todo tempo com o berço
	// ocupado conta como espera por berço — o ganho de mais capacidade é
	// o maior possível.
	Otimista Prontidao = iota
	// Conservadora: o navio só estava pronto depois de atracar o último
	// navio que o ultrapassou. O ganho é o menor plausível.
	Conservadora
)

func (p Prontidao) String() string {
	if p == Conservadora {
		return "conservadora"
	}
	return "otimista"
}

// Navio é uma atracação vista pela fila. Os instantes são horas desde a
// origem (o início do período analisado); antes dela, negativos.
type Navio struct {
	ID, Berco, Carga string

	Chegada, Atracacao, Desatracacao float64 // reais

	// O tempo no berço dividido como a ANTAQ divide: T2 atracado
	// esperando começar, T3 operando, T4 esperando desatracar. Somam
	// Desatracacao − Atracacao. Sem as datas de operação, tudo vai para
	// T3.
	T2, T3, T4 float64

	// Excedente marca o navio que o dado mostra atracado com todas as
	// posições do berço ocupadas — quase sempre erro de registro de
	// minutos. Ele não disputa posição no replay, para que o resto da
	// fila siga como foi.
	Excedente bool

	// Pronto é quando o navio pode começar a ocupar o berço; Manobra é
	// quanto o berço fica reservado para ele antes de atracar (prático,
	// maré, amarração). Um valor por leitura de prontidão. Os dois são
	// tirados do dado de forma que, sem cenário, o navio atraca na hora
	// em que atracou de verdade.
	Pronto, Manobra [2]float64
}

// NoBerco devolve as horas do navio no berço.
func (n *Navio) NoBerco() float64 { return n.T2 + n.T3 + n.T4 }

// DadosGrupo é tudo o que a fila de um grupo de berços precisa.
type DadosGrupo struct {
	Grupo  Grupo
	Origem time.Time
	Fim    float64 // fim do período, em horas desde a origem

	// Posicoes é quantos navios cada berço comporta ao mesmo tempo: o
	// menor número que cobre 98% do tempo em que o berço esteve ocupado.
	// Quase sempre 1; píeres com dois lados dão 2. Não se usa o máximo
	// porque uma sobreposição de minutos, erro de registro, dobraria a
	// capacidade do berço.
	Posicoes map[string]int
	// Navios em ordem de atracação real.
	Navios []Navio
	// ManobraTipica é a mediana do intervalo entre um navio sair e o
	// próximo atracar quando havia fila, por leitura de prontidão.
	// LimiteManobra é o percentil 90 desse intervalo: até ele, o
	// intervalo é manobra (canal, maré, prático) e se repete em qualquer
	// cenário; acima, o navio não estava pronto, e fica pronto
	// ManobraTipica antes de atracar.
	ManobraTipica, LimiteManobra [2]float64
}

// NoPeriodo diz se o navio chegou dentro do período analisado.
func (d *DadosGrupo) NoPeriodo(n *Navio) bool { return n.Chegada >= 0 && n.Chegada < d.Fim }

// MontarGrupo separa as atracações do grupo e deriva prontidão, manobra
// e posições.
func MontarGrupo(g Grupo, as []antaq.Atracacao, periodo Janela) *DadosGrupo {
	d := &DadosGrupo{
		Grupo:    g,
		Origem:   periodo.Inicio.Time,
		Fim:      periodo.Fim.Sub(periodo.Inicio.Time).Hours(),
		Posicoes: map[string]int{},
	}
	noGrupo := map[string]bool{}
	for _, b := range g.Bercos {
		noGrupo[b] = true
	}
	h := func(t time.Time) float64 { return t.Sub(d.Origem).Hours() }
	for i := range as {
		a := &as[i]
		if !noGrupo[a.Berco] || !a.Completa() {
			continue
		}
		n := Navio{
			ID: a.ID, Berco: a.Berco, Carga: a.Mercadoria,
			Chegada: h(a.Chegada), Atracacao: h(a.Atracacao), Desatracacao: h(a.Desatracacao),
		}
		ini, fim := h(a.InicioOp), h(a.FimOp)
		if !a.InicioOp.IsZero() && !a.FimOp.IsZero() && n.Atracacao <= ini && ini <= fim && fim <= n.Desatracacao {
			n.T2, n.T3, n.T4 = ini-n.Atracacao, fim-ini, n.Desatracacao-fim
		} else {
			n.T3 = n.Desatracacao - n.Atracacao
		}
		d.Navios = append(d.Navios, n)
	}
	sort.SliceStable(d.Navios, func(i, j int) bool { return d.Navios[i].Atracacao < d.Navios[j].Atracacao })

	d.contarPosicoes()
	lib := d.liberacoes()
	for _, p := range []Prontidao{Otimista, Conservadora} {
		prontos := d.prontidao(p)
		d.ManobraTipica[p], d.LimiteManobra[p] = manobras(d.Navios, prontos, lib)
		for i := range d.Navios {
			n := &d.Navios[i]
			r := prontos[i]
			troca := n.Atracacao - math.Max(r, lib[i])
			if troca > d.LimiteManobra[p] {
				// o berço ficou livre e o navio demorou mais que quase
				// qualquer manobra: não estava pronto
				n.Pronto[p], n.Manobra[p] = n.Atracacao-d.ManobraTipica[p], d.ManobraTipica[p]
			} else {
				n.Pronto[p], n.Manobra[p] = r, troca
			}
		}
	}
	return d
}

// toleranciaPosicoes é a fração do tempo ocupado que pode passar das
// posições do berço sem que isso conte como mais uma posição.
const toleranciaPosicoes = 0.02

// contarPosicoes acha quantas posições cada berço tem.
func (d *DadosGrupo) contarPosicoes() {
	type ev struct {
		t     float64
		delta int
	}
	por := map[string][]ev{}
	for _, n := range d.Navios {
		por[n.Berco] = append(por[n.Berco], ev{n.Atracacao, 1}, ev{n.Desatracacao, -1})
	}
	for _, b := range d.Grupo.Bercos {
		evs := por[b]
		// saída antes de entrada no mesmo instante: troca de navio não é
		// sobreposição
		sort.Slice(evs, func(i, j int) bool {
			if evs[i].t == evs[j].t {
				return evs[i].delta < evs[j].delta
			}
			return evs[i].t < evs[j].t
		})
		tempo := map[int]float64{} // horas com k navios no berço
		ocupado := 0.0
		cur := 0
		for i, e := range evs {
			if i > 0 && cur > 0 {
				dt := e.t - evs[i-1].t
				tempo[cur] += dt
				ocupado += dt
			}
			cur += e.delta
		}
		k := 1
		for {
			acima := 0.0
			for n, h := range tempo {
				if n > k {
					acima += h
				}
			}
			if ocupado == 0 || acima/ocupado < toleranciaPosicoes {
				break
			}
			k++
		}
		d.Posicoes[b] = k
	}
}

// liberacoes devolve, para cada navio, quando a posição que ele ocupou
// ficou livre (−Inf se estava livre desde sempre, ou se o navio é
// excedente). Marca os excedentes.
func (d *DadosGrupo) liberacoes() []float64 {
	livres := map[string]*minHeap{}
	for b, k := range d.Posicoes {
		h := &minHeap{}
		for range k {
			*h = append(*h, math.Inf(-1))
		}
		livres[b] = h
	}
	lib := make([]float64, len(d.Navios))
	for i := range d.Navios {
		n := &d.Navios[i]
		h := livres[n.Berco]
		if (*h)[0] > n.Atracacao {
			n.Excedente = true
			lib[i] = math.Inf(-1)
			continue
		}
		lib[i] = heap.Pop(h).(float64)
		heap.Push(h, n.Desatracacao)
	}
	return lib
}

// prontidao devolve quando cada navio passa a poder atracar.
func (d *DadosGrupo) prontidao(p Prontidao) []float64 {
	r := make([]float64, len(d.Navios))
	for i, n := range d.Navios {
		r[i] = n.Chegada
		if p != Conservadora {
			continue
		}
		// navios em ordem de atracação: quem atracou antes de i e chegou
		// depois dele o ultrapassou
		for j := range i {
			if m := d.Navios[j]; m.Chegada > n.Chegada && m.Atracacao > r[i] {
				r[i] = m.Atracacao
			}
		}
	}
	return r
}

// manobras devolve a mediana e o percentil 90 do intervalo entre a
// posição liberar e o navio seguinte atracar, contando só quando havia
// fila (o navio já estava pronto quando a posição liberou).
func manobras(ns []Navio, prontos, lib []float64) (mediana, p90 float64) {
	var xs []float64
	for i, n := range ns {
		if lib[i] > prontos[i] {
			xs = append(xs, n.Atracacao-lib[i])
		}
	}
	if len(xs) == 0 {
		return 0, 0
	}
	sort.Float64s(xs)
	return medianaDe(xs), xs[int(math.Ceil(0.9*float64(len(xs))))-1]
}

type minHeap []float64

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(float64)) }
func (h *minHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
