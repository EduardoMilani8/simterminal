package porto

import (
	"math"
	"sort"
)

// Previsão da espera de um navio no momento em que ele chega, usando só
// o que se sabia naquele instante: quem estava fundeado, quem estava no
// berço e como a fila andou nos dias anteriores. Nada do futuro entra —
// é o que um painel do porto poderia mostrar ao vivo.

// Estado é o que se sabe do grupo no instante t.
type Estado struct {
	Fila     int     // navios que chegaram e ainda não atracaram
	Vazao    float64 // navios que saíram do berço por hora, nos últimos 30 dias
	Recente  float64 // mediana da espera de quem atracou nos últimos 30 dias, em horas
	Restante float64 // horas de berço que faltam para quem está atracado, pela mediana recente
}

const janelaRecente = 30 * 24.0

// EstadoEm calcula o estado do grupo no instante t, sem olhar nada
// posterior a t. ignorar é o índice do próprio navio (−1 se nenhum).
func EstadoEm(d *DadosGrupo, t float64, ignorar int) Estado {
	var e Estado
	var esperas, servicos []float64
	saidas := 0
	for j := range d.Navios {
		if j == ignorar {
			continue
		}
		n := &d.Navios[j]
		if n.Chegada < t && n.Atracacao > t {
			e.Fila++
		}
		if n.Atracacao < t && n.Atracacao >= t-janelaRecente {
			esperas = append(esperas, n.Atracacao-n.Chegada)
		}
		if n.Desatracacao < t && n.Desatracacao >= t-3*janelaRecente {
			servicos = append(servicos, n.NoBerco())
		}
		if n.Desatracacao < t && n.Desatracacao >= t-janelaRecente {
			saidas++
		}
	}
	e.Vazao = float64(saidas) / janelaRecente
	e.Recente = medianaDe(esperas)
	svc := medianaDe(servicos)
	for j := range d.Navios {
		if n := &d.Navios[j]; j != ignorar && n.Atracacao <= t && n.Desatracacao > t {
			e.Restante += math.Max(0, svc-(t-n.Atracacao))
		}
	}
	return e
}

// filaVazao é a espera se a fila andar no ritmo recente: 12 navios na
// frente e 10 saindo por mês dão pouco mais de um mês.
func (e Estado) filaVazao() float64 {
	if e.Vazao <= 0 {
		return e.Recente
	}
	return (float64(e.Fila) + 0.5) / e.Vazao
}

func (e Estado) atributos() []float64 { return []float64{1, e.filaVazao(), e.Recente} }

// Metodo é uma forma de prever a espera.
type Metodo int

const (
	// PelaMediana repete a espera mediana dos últimos 30 dias: o que um
	// relatório mensal do porto diria.
	PelaMediana Metodo = iota
	// PelaFila divide a fila pela vazão recente.
	PelaFila
	// Regressao combina fila ÷ vazão e mediana recente com pesos
	// reaprendidos a cada navio, com os navios dos 12 meses anteriores
	// que já tinham atracado. Pesos fixos, aprendidos num ano, erram no
	// seguinte: o regime da fila muda de uma safra para outra.
	Regressao
)

// Metodos são todos os métodos, na ordem em que aparecem nos relatórios.
var Metodos = []Metodo{PelaMediana, PelaFila, Regressao}

var nomesMetodo = []string{"mediana dos últimos 30 dias", "fila ÷ vazão recente", "regressão móvel (12 meses)"}

func (m Metodo) String() string { return nomesMetodo[m] }

const janelaRegressao = 365 * 24.0

// Previsor prevê a espera dos navios de um grupo, cada um no instante em
// que chegou.
type Previsor struct {
	d       *DadosGrupo
	estados []Estado // estado do grupo na chegada de cada navio
}

// NovoPrevisor calcula o estado na chegada de cada navio.
func NovoPrevisor(d *DadosGrupo) *Previsor {
	p := &Previsor{d: d, estados: make([]Estado, len(d.Navios))}
	for i := range d.Navios {
		p.estados[i] = EstadoEm(d, d.Navios[i].Chegada, i)
	}
	return p
}

// Prever devolve a espera prevista, em horas, para o navio i na chegada.
func (p *Previsor) Prever(m Metodo, i int) float64 {
	e := p.estados[i]
	switch m {
	case PelaMediana:
		return e.Recente
	case PelaFila:
		return e.filaVazao()
	}
	t := p.d.Navios[i].Chegada
	var X [][]float64
	var y []float64
	for j := range p.d.Navios {
		n := &p.d.Navios[j]
		// só quem já tinha atracado: a espera dele era conhecida em t
		if j != i && n.Chegada >= t-janelaRegressao && n.Atracacao < t {
			X = append(X, p.estados[j].atributos())
			y = append(y, n.Atracacao-n.Chegada)
		}
	}
	beta := minimosQuadrados(X, y)
	if beta == nil {
		return e.filaVazao()
	}
	w := 0.0
	for k, x := range e.atributos() {
		w += beta[k] * x
	}
	return math.Max(0, w)
}

// Avaliacao é o erro de um método nos navios de uma janela.
type Avaliacao struct {
	Metodo      Metodo
	Navios      int
	ErroMedio   float64 // média de |previsto − real|, horas
	ErroMediano float64
	Vies        float64 // média de previsto − real: positivo = superestima
	Dentro48h   float64 // fração com erro de até dois dias
}

// Avaliar mede cada método nos navios que chegaram na janela.
func (p *Previsor) Avaliar(j Janela) []Avaliacao {
	ini := j.Inicio.Sub(p.d.Origem).Hours()
	fim := j.Fim.Sub(p.d.Origem).Hours()
	var out []Avaliacao
	for _, m := range Metodos {
		a := Avaliacao{Metodo: m}
		var abs []float64
		for i := range p.d.Navios {
			n := &p.d.Navios[i]
			if n.Chegada < ini || n.Chegada >= fim {
				continue
			}
			erro := p.Prever(m, i) - (n.Atracacao - n.Chegada)
			abs = append(abs, math.Abs(erro))
			a.Vies += erro
			if math.Abs(erro) <= 48 {
				a.Dentro48h++
			}
		}
		a.Navios = len(abs)
		if a.Navios > 0 {
			sort.Float64s(abs)
			soma := 0.0
			for _, x := range abs {
				soma += x
			}
			a.ErroMedio = soma / float64(a.Navios)
			a.ErroMediano = medianaDe(abs)
			a.Vies /= float64(a.Navios)
			a.Dentro48h /= float64(a.Navios)
		}
		out = append(out, a)
	}
	return out
}

// Melhor devolve o método de menor erro médio.
func Melhor(as []Avaliacao) Metodo {
	best := 0
	for i, a := range as {
		if a.Navios > 0 && a.ErroMedio < as[best].ErroMedio {
			best = i
		}
	}
	return as[best].Metodo
}

// minimosQuadrados resolve (XᵀX)β = Xᵀy por eliminação de Gauss. Devolve
// nil se houver menos de 30 observações ou o sistema for singular.
func minimosQuadrados(X [][]float64, y []float64) []float64 {
	if len(X) < 30 {
		return nil
	}
	k := len(X[0])
	A := make([][]float64, k)
	for i := range A {
		A[i] = make([]float64, k+1)
	}
	for r, x := range X {
		for i := range k {
			for j := range k {
				A[i][j] += x[i] * x[j]
			}
			A[i][k] += x[i] * y[r]
		}
	}
	for c := range k {
		p := c
		for r := c + 1; r < k; r++ {
			if math.Abs(A[r][c]) > math.Abs(A[p][c]) {
				p = r
			}
		}
		if math.Abs(A[p][c]) < 1e-9 {
			return nil
		}
		A[c], A[p] = A[p], A[c]
		for r := range k {
			if r == c {
				continue
			}
			f := A[r][c] / A[c][c]
			for j := c; j <= k; j++ {
				A[r][j] -= f * A[c][j]
			}
		}
	}
	beta := make([]float64, k)
	for i := range k {
		beta[i] = A[i][k] / A[i][i]
	}
	return beta
}
