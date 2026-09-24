package porto

// Capacidade responde a pergunta de quem monta a agenda de atracações:
// quantos navios por mês dá para aceitar sem a espera passar da meta?
//
// O replay é rodado com a demanda multiplicada (os mesmos navios,
// chegando mais juntos ou mais espaçados) até achar o maior ritmo em que
// a espera mediana fica dentro da meta.
type Capacidade struct {
	Cenario     Cenario
	HojeMes     float64 // navios por mês no período, como foi
	HojeMediana float64 // espera mediana real no período, horas
	MetaHoras   float64
	Otimista    float64 // navios/mês que cabem na meta; 0 se nem com a menor demanda testada
	Conservador float64
	// Mediana da espera em cada nível de demanda testado, para mostrar a
	// curva: a espera não cresce em linha reta, explode perto do limite.
	Curva []PontoCurva
}

// PontoCurva é a espera mediana num nível de demanda.
type PontoCurva struct {
	NaviosMes                    float64
	MedianaOtimista, MedianaCons float64
}

// Niveis de demanda testados, em múltiplos da demanda real.
var Niveis = func() []float64 {
	var xs []float64
	for f := 0.30; f <= 2.001; f += 0.05 {
		xs = append(xs, f)
	}
	return xs
}()

const horasMes = 24 * 365.0 / 12

// CalcularCapacidade acha a capacidade do grupo no cenário para a meta de
// espera mediana, em horas.
func CalcularCapacidade(d *DadosGrupo, sc Cenario, metaHoras float64) Capacidade {
	real := ResumoReal(d)
	hoje := float64(real.Navios) / (d.Fim / horasMes)
	c := Capacidade{Cenario: sc, HojeMes: hoje, HojeMediana: real.EsperaMediana, MetaHoras: metaHoras}
	cabeOt, cabeCons := true, true
	for _, f := range Niveis {
		s := sc
		s.Demanda = Fator(f)
		ot := Replay{Dados: d, Cenario: s, Modo: Otimista}.Rodar().Resumo()
		co := Replay{Dados: d, Cenario: s, Modo: Conservadora}.Rodar().Resumo()
		c.Curva = append(c.Curva, PontoCurva{hoje * f, ot.EsperaMediana, co.EsperaMediana})
		// só conta enquanto todos os níveis abaixo também couberam: a
		// curva de um ano real tem soluços, e um ponto isolado dentro da
		// meta depois de um fora não é capacidade
		cabeOt = cabeOt && ot.EsperaMediana <= metaHoras
		cabeCons = cabeCons && co.EsperaMediana <= metaHoras
		if cabeOt {
			c.Otimista = hoje * f
		}
		if cabeCons {
			c.Conservador = hoje * f
		}
	}
	return c
}
