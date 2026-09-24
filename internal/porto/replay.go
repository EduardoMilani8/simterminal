package porto

import (
	"container/heap"
	"math"

	"github.com/eduardomilani8/simterminal/internal/des"
)

// Replay refaz o período de um grupo de berços sob um cenário.
//
// Os navios que atracaram antes da origem ficam como estavam: ocupam o
// berço até a hora real de saída. A partir da origem, cada navio fica
// pronto, espera posição livre, manobra e ocupa o berço pelo seu tempo
// real — ajustado pelo cenário.
type Replay struct {
	Dados   *DadosGrupo
	Cenario Cenario
	Modo    Prontidao
	// NovaChegada, se definida, dá a chegada instruída de cada navio (por
	// índice em Dados.Navios): é assim que se testa chegada just-in-time.
	// Só vale para navios que atracaram depois da origem e não pode ser
	// antes da chegada real.
	NovaChegada func(i int) float64
	// Chamada, se definida, põe os navios numa fila virtual: só vêm para
	// o fundeadouro quando chamados. Vale com alocação por berço ou
	// compartilhada.
	Chamada *Chamada
}

// Chamada é a regra da fila virtual, o equivalente do pátio regulador de
// caminhões: cada fila mantém Reserva navios fundeados ou a caminho, e
// chama o próximo da vez com Antecedencia horas. O navio que não foi
// chamado espera no mar, reduzindo a velocidade, e não no fundeadouro.
//
// Reserva zero calcula a reserva de cada fila: quantos navios ela
// atende durante a antecedência, mais um. Com reserva menor que isso o
// berço fica parado esperando o chamado chegar — em berço de contêiner,
// onde o navio fica 18 h, uma reserva de 1 com chamada de 48 h derruba a
// capacidade.
type Chamada struct {
	Reserva      int
	Antecedencia float64
}

// reservaPara devolve a reserva de uma fila com k posições e tempo
// mediano de berço svc.
func (c *Chamada) reservaPara(k int, svc float64) int {
	if c.Reserva > 0 {
		return c.Reserva
	}
	if svc <= 0 {
		return k + 1
	}
	return int(math.Ceil(float64(k)*c.Antecedencia/svc)) + 1
}

// Resultado é o que aconteceu com cada navio no replay, alinhado com
// Dados.Navios.
type Resultado struct {
	Dados    *DadosGrupo
	Cenario  Cenario
	Modo     Prontidao
	Chegada  []float64 // chegada efetiva (com demanda e chegada instruída)
	Atracou  []float64
	Saiu     []float64
	Posicoes int     // posições de atracação do grupo no cenário
	Janela   float64 // horas medidas (o período, comprimido pela demanda)
}

// Simulado diz se o navio i foi refeito pelo replay (atracou depois da
// origem) ou ficou como no dado.
func (r Resultado) Simulado(i int) bool { return r.Dados.Navios[i].Atracacao >= 0 }

// Rodar executa o replay.
func (rp Replay) Rodar() Resultado {
	d := rp.Dados
	sc := rp.Cenario
	operacao, ocioso, demanda := sc.Fatores()
	n := len(d.Navios)
	res := Resultado{
		Dados: d, Cenario: sc, Modo: rp.Modo,
		Chegada: make([]float64, n), Atracou: make([]float64, n), Saiu: make([]float64, n),
		Janela: d.Fim / demanda,
	}
	for _, k := range d.Posicoes {
		res.Posicoes += k
	}
	res.Posicoes += sc.BercosExtra

	pronto := make([]float64, n)
	servico := make([]float64, n)
	for i := range d.Navios {
		nv := &d.Navios[i]
		if nv.Atracacao < 0 {
			res.Chegada[i], res.Atracou[i], res.Saiu[i] = nv.Chegada, nv.Atracacao, nv.Desatracacao
			continue
		}
		c, p := nv.Chegada, nv.Pronto[rp.Modo]
		if c >= 0 && demanda != 1 {
			c, p = c/demanda, c/demanda+(p-c)
		}
		if rp.NovaChegada != nil {
			if nc := rp.NovaChegada(i); nc > c {
				c = nc
				p = math.Max(p, nc)
			}
		}
		res.Chegada[i], pronto[i] = c, p
		servico[i] = (nv.T2+nv.T4)*ocioso + nv.T3*operacao
	}

	switch sc.Alocacao {
	case AlocCompartilhada:
		todas := map[string]int{"grupo": res.Posicoes}
		rp.filas(&res, pronto, servico, todas, func(int) string { return "grupo" })
	case AlocPorBerco:
		rp.filas(&res, pronto, servico, d.Posicoes, func(i int) string { return d.Navios[i].Berco })
	default:
		rp.fixa(&res, pronto, servico)
	}
	return res
}

// fixa: cada berço atende a sua sequência real de navios, na mesma
// ordem. Se o próximo da sequência não está pronto, o berço espera por
// ele — como aconteceu.
func (rp Replay) fixa(res *Resultado, pronto, servico []float64) {
	d := rp.Dados
	livres := map[string]*minHeap{}
	for b, k := range d.Posicoes {
		h := &minHeap{}
		for range k {
			*h = append(*h, math.Inf(-1))
		}
		livres[b] = h
	}
	for i := range d.Navios {
		nv := &d.Navios[i]
		if nv.Excedente {
			if nv.Atracacao >= 0 {
				res.Atracou[i] = pronto[i] + nv.Manobra[rp.Modo]
				res.Saiu[i] = res.Atracou[i] + servico[i]
			}
			continue
		}
		h := livres[nv.Berco]
		lib := heap.Pop(h).(float64)
		if nv.Atracacao >= 0 {
			inicio := math.Max(pronto[i], lib)
			res.Atracou[i] = inicio + nv.Manobra[rp.Modo]
			res.Saiu[i] = res.Atracou[i] + servico[i]
		}
		heap.Push(h, res.Saiu[i])
	}
}

// filas: cada fila (o grupo inteiro, ou um berço) tem suas posições;
// quando uma posição libera, vai o navio pronto de maior prioridade
// daquela fila.
func (rp Replay) filas(res *Resultado, pronto, servico []float64, posicoes map[string]int, filaDe func(int) string) {
	d := rp.Dados
	e := des.New(0) // o replay não sorteia nada; a semente não importa
	chave := func(i int) float64 {
		if rp.Cenario.Ordem == OrdemChegada {
			return res.Chegada[i]
		}
		return d.Navios[i].Atracacao
	}
	filas := map[string]*filaNavios{}
	livres := map[string]int{}
	for f, k := range posicoes {
		filas[f] = &filaNavios{chave: chave}
		livres[f] = k
	}

	// fila virtual: quem ainda não foi chamado, e quantos a fila tem
	// fundeados ou a caminho
	virtual := map[string]*filaNavios{}
	reserva := map[string]int{}
	limite := map[string]int{}
	for f := range posicoes {
		virtual[f] = &filaNavios{chave: chave}
	}
	if rp.Chamada != nil {
		svcs := map[string][]float64{}
		for i := range d.Navios {
			svcs[filaDe(i)] = append(svcs[filaDe(i)], d.Navios[i].NoBerco())
		}
		for f, k := range posicoes {
			limite[f] = rp.Chamada.reservaPara(k, medianaDe(svcs[f]))
		}
	}
	var despachar func(f string)
	// chamar chama da fila virtual até completar a reserva. Quem pode
	// chegar agora entra direto na fila física (devolve true); quem
	// chega depois vira evento.
	chamar := func(f string) bool {
		ch := rp.Chamada
		imediato := false
		for ch != nil && reserva[f] < limite[f] && virtual[f].Len() > 0 {
			i := heap.Pop(virtual[f]).(int)
			reserva[f]++
			chega := math.Max(pronto[i], e.Now()+ch.Antecedencia)
			res.Chegada[i] = math.Max(res.Chegada[i], chega)
			if chega <= e.Now() {
				heap.Push(filas[f], i)
				imediato = true
				continue
			}
			e.ScheduleAt(chega, "chamado chega", func() { heap.Push(filas[f], i); despachar(f) })
		}
		return imediato
	}
	despachar = func(f string) {
		for {
			for livres[f] > 0 && filas[f].Len() > 0 {
				i := heap.Pop(filas[f]).(int)
				livres[f]--
				reserva[f]--
				res.Atracou[i] = e.Now() + d.Navios[i].Manobra[rp.Modo]
				res.Saiu[i] = res.Atracou[i] + servico[i]
				e.ScheduleAt(res.Saiu[i], "desatraca", func() { livres[f]++; despachar(f) })
			}
			if !chamar(f) {
				return
			}
		}
	}

	// quem já estava no berço na origem segura a posição até sair
	for i := range d.Navios {
		if nv := &d.Navios[i]; nv.Atracacao < 0 && nv.Desatracacao > 0 && !nv.Excedente {
			f := filaDe(i)
			livres[f]--
			e.ScheduleAt(nv.Desatracacao, "desatraca (antes da origem)", func() { livres[f]++; despachar(f) })
		}
	}
	for i := range d.Navios {
		if !res.Simulado(i) {
			continue
		}
		if d.Navios[i].Excedente { // não disputa posição, como em fixa
			res.Atracou[i] = pronto[i] + d.Navios[i].Manobra[rp.Modo]
			res.Saiu[i] = res.Atracou[i] + servico[i]
			continue
		}
		f := filaDe(i)
		if rp.Chamada == nil || d.Navios[i].Chegada < 0 {
			// sem fila virtual, ou já fundeado na origem: vem direto
			reserva[f]++
			e.ScheduleAt(math.Max(0, pronto[i]), "pronto", func() { heap.Push(filas[f], i); despachar(f) })
			continue
		}
		// entra na fila virtual quando estaria a Antecedencia horas de chegar
		entra := math.Max(0, pronto[i]-rp.Chamada.Antecedencia)
		e.ScheduleAt(entra, "fila virtual", func() { heap.Push(virtual[f], i); despachar(f) })
	}
	e.Run(0)
}

type filaNavios struct {
	ids   []int
	chave func(int) float64
}

func (f filaNavios) Len() int { return len(f.ids) }
func (f filaNavios) Less(a, b int) bool {
	ka, kb := f.chave(f.ids[a]), f.chave(f.ids[b])
	if ka == kb {
		return f.ids[a] < f.ids[b]
	}
	return ka < kb
}
func (f filaNavios) Swap(a, b int) { f.ids[a], f.ids[b] = f.ids[b], f.ids[a] }
func (f *filaNavios) Push(x any)   { f.ids = append(f.ids, x.(int)) }
func (f *filaNavios) Pop() any {
	x := f.ids[len(f.ids)-1]
	f.ids = f.ids[:len(f.ids)-1]
	return x
}
