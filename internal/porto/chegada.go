package porto

import "math"

// Chegada just-in-time: em vez de o navio correr até o porto e ficar
// dias fundeado, o porto informa quando o berço deve liberar e o navio
// ajusta a velocidade para chegar perto dessa hora. É o que a IMO chama
// de Just In Time Arrival. Ganha-se em combustível, emissões e risco no
// fundeadouro; o custo aparece se o navio chegar tarde e o berço ficar
// parado esperando por ele — e isso atrasa todo mundo atrás.
//
// Aqui a política é testada no replay com alocação por berço: cada navio
// vai para o berço em que atracou de fato, e o berço atende o primeiro
// pronto da sua fila — se o navio instruído chegar tarde, o berço não
// fica parado esperando, atende o seguinte. A comparação é contra o
// mesmo replay sem instrução, para isolar o efeito da chegada.

// PoliticaChegada define quando cada navio vem para o fundeadouro.
type PoliticaChegada struct {
	Nome string
	// Hora marcada: o navio chega na espera prevista menos a folga.
	Folga   float64
	Oraculo bool // usa a espera da referência: o limite do que dá para ganhar
	// Fila virtual: o navio só vem quando chamado. Excludente com a hora
	// marcada.
	Chamada *Chamada
}

// ResultadoChegada mede uma política nos navios do período.
type ResultadoChegada struct {
	Politica     PoliticaChegada
	FundeadoDias float64 // navio-dias parados entre a chegada e a atracação
	Economia     float64 // fração do fundeio da referência evitada
	AtrasoMedio  float64 // horas: quanto a atracação ficou depois da referência, na média
	Atrasados    float64 // fração dos navios que atracaram mais de 6 h depois da referência
}

// Politicas padrão: o limite teórico, hora marcada pela previsão e fila
// virtual.
func Politicas() []PoliticaChegada {
	return []PoliticaChegada{
		{Nome: "oráculo (sabe quando o berço libera)", Oraculo: true},
		{Nome: "hora marcada: previsão − 2 dias", Folga: 48},
		{Nome: "hora marcada: previsão − 4 dias", Folga: 96},
		{Nome: "fila virtual: 1 de reserva por berço, chamada 48 h", Chamada: &Chamada{Reserva: 1, Antecedencia: 48}},
		{Nome: "fila virtual: 2 de reserva por berço, chamada 48 h", Chamada: &Chamada{Reserva: 2, Antecedencia: 48}},
		{Nome: "fila virtual: reserva que cobre as 48 h", Chamada: &Chamada{Antecedencia: 48}},
	}
}

// AvaliarChegada roda cada política. A primeira linha é o replay sem
// instrução, com a mesma regra de alocação: a referência.
func AvaliarChegada(d *DadosGrupo, p *Previsor, m Metodo, pols []PoliticaChegada) []ResultadoChegada {
	sc := Base()
	sc.Nome, sc.Alocacao = "por berço", AlocPorBerco
	base := Replay{Dados: d, Cenario: sc, Modo: Otimista}.Rodar()

	ref := ResultadoChegada{Politica: PoliticaChegada{Nome: "chegada como foi"}}
	for i := range d.Navios {
		if d.NoPeriodo(&d.Navios[i]) {
			ref.FundeadoDias += (base.Atracou[i] - base.Chegada[i]) / 24
		}
	}
	out := []ResultadoChegada{ref}

	previsto := map[int]float64{}
	for i := range d.Navios {
		if n := &d.Navios[i]; d.NoPeriodo(n) && n.Atracacao >= 0 {
			previsto[i] = p.Prever(m, i)
		}
	}

	for _, pol := range pols {
		rp := Replay{Dados: d, Cenario: sc, Modo: Otimista, Chamada: pol.Chamada}
		rp.NovaChegada = func(i int) float64 {
			n := &d.Navios[i]
			w, ok := previsto[i]
			if !ok {
				return n.Chegada
			}
			if pol.Oraculo {
				// sabe quando o berço vai liberar para ele na referência
				w = base.Atracou[i] - base.Chegada[i] - n.Manobra[Otimista]
			}
			return n.Chegada + math.Max(0, w-pol.Folga)
		}
		if pol.Chamada != nil {
			rp.NovaChegada = nil
		}
		r := rp.Rodar()
		rc := ResultadoChegada{Politica: pol}
		n, atrasados := 0, 0
		for i := range d.Navios {
			if !d.NoPeriodo(&d.Navios[i]) {
				continue
			}
			n++
			rc.FundeadoDias += (r.Atracou[i] - r.Chegada[i]) / 24
			atraso := r.Atracou[i] - base.Atracou[i]
			rc.AtrasoMedio += atraso
			if atraso > 6 {
				atrasados++
			}
		}
		if n > 0 {
			rc.AtrasoMedio /= float64(n)
			rc.Atrasados = float64(atrasados) / float64(n)
		}
		if ref.FundeadoDias > 0 {
			rc.Economia = 1 - rc.FundeadoDias/ref.FundeadoDias
		}
		out = append(out, rc)
	}
	return out
}
