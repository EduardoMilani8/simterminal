package report

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/eduardomilani8/simterminal/internal/porto"
)

// Dias formata horas de espera de navio: "20 h" até dois dias, depois
// "11,9 d". Navio espera em dias; minuto aqui é ruído.
func Dias(h float64) string {
	var s string
	if math.Abs(h) < 48 {
		s = fmt.Sprintf("%.0f h", h)
	} else {
		s = decimal(h/24, 1) + " d"
	}
	if s == "-0 h" {
		return "0 h"
	}
	return strings.Replace(s, "-", "−", 1)
}

// FaixaDias formata um intervalo de horas: "8,2 a 10,4 d", ou um valor só
// quando as duas pontas coincidem no arredondamento.
func FaixaDias(lo, hi float64) string {
	a, b := Dias(lo), Dias(hi)
	if a == b {
		return a
	}
	ua, ub := unidade(a), unidade(b)
	if ua == ub {
		return strings.TrimSuffix(a, " "+ua) + " a " + b
	}
	return a + " a " + b
}

func unidade(s string) string { return s[strings.LastIndexByte(s, ' ')+1:] }

func decimal(x float64, casas int) string {
	return strings.Replace(fmt.Sprintf("%.*f", casas, x), ".", ",", 1)
}

// Milhar formata um inteiro com ponto de milhar: 12345 → "12.345".
func Milhar(x float64) string {
	s := fmt.Sprintf("%.0f", math.Abs(x))
	var b strings.Builder
	if x < 0 && s != "0" {
		b.WriteString("−")
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func faixaMilhar(lo, hi float64) string {
	a, b := Milhar(lo), Milhar(hi)
	if a == b {
		return a
	}
	return a + " a " + b
}

// faixaPercent formata um intervalo de variações: "−26 a −2%".
func faixaPercent(lo, hi float64) string {
	a, b := percentLimpo(lo), percentLimpo(hi)
	s := strings.TrimSuffix(a, "%") + " a " + b
	if a == b {
		s = a
	}
	return s
}

func periodoLabel(c *porto.Config) string {
	ini, fim := c.Periodo.Inicio.Time, c.Periodo.Fim.Time.AddDate(0, 0, -1)
	if ini.Year() == fim.Year() && ini.YearDay() == 1 && fim.Month() == 12 && fim.Day() == 31 {
		return fmt.Sprint(ini.Year())
	}
	return ini.Format("02/01/2006") + " a " + fim.Format("02/01/2006")
}

func nomeCarga(nomes map[string]string, cod string) string {
	if n, ok := nomes[cod]; ok && n != "" {
		return n
	}
	return cod
}

// WritePortoDiagnostico escreve o retrato do porto a partir do dado real.
func WritePortoDiagnostico(w io.Writer, c *porto.Config, ds []porto.Diagnostico, nomes map[string]string) {
	fmt.Fprintf(w, "%s — navios que chegaram em %s (atracações da ANTAQ)\n\n", c.Porto, periodoLabel(c))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  grupo\tberços\tnavios\tespera mediana\tp90\tnavio-dias\tocupação\tberço cheio*\tultrapassados\n")
	for _, d := range ds {
		fmt.Fprintf(tw, "  %s\t%d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n", d.Grupo, d.Bercos, d.Real.Navios,
			Dias(d.Real.EsperaMediana), Dias(d.Real.EsperaP90), Milhar(d.Real.NavioDias),
			Percent(d.Real.Ocupacao), Percent(d.BercoCheio), Percent(d.Ultrapassados))
	}
	tw.Flush()
	fmt.Fprintln(w, "\n  * berço cheio: parte do tempo de espera em que todos os berços do grupo estavam ocupados.")
	fmt.Fprintln(w, "    ultrapassados: navios que viram alguém chegar depois e atracar antes.")
	if c.CustoNavioDia > 0 {
		total := 0.0
		for _, d := range ds {
			total += d.Real.NavioDias
		}
		fmt.Fprintf(w, "\n  %s navio-dias de espera × US$ %s/dia = US$ %s no período.\n",
			Milhar(total), Milhar(c.CustoNavioDia), Milhar(total*c.CustoNavioDia))
	}

	for _, d := range ds {
		fmt.Fprintf(w, "\n  %s\n", d.Grupo)
		fmt.Fprintf(w, "    %s.\n", d.Leitura())
		fmt.Fprintf(w, "    no berço: %s por navio, %s disso atracado sem operar; entre um navio e outro, %s.\n",
			Dias(d.NoBercoMedio), Percent(d.Ocioso), Dias(d.ManobraTipica))
		if d.Posicoes != d.Bercos {
			fmt.Fprintf(w, "    %d posições de atracação em %s (há berço que recebe dois navios ao mesmo tempo).\n", d.Posicoes, plural(d.Bercos, "berço", "berços"))
		}
		if len(d.Cargas) > 0 {
			// códigos diferentes podem ter o mesmo nome curto (três NCM
			// de adubo viram "Adubos"): soma pelo nome
			var porNome []porto.Parcela
			pos := map[string]int{}
			for _, p := range d.Cargas {
				n := nomeCarga(nomes, p.Codigo)
				if i, ok := pos[n]; ok {
					porNome[i].Fracao += p.Fracao
					continue
				}
				pos[n] = len(porNome)
				porNome = append(porNome, porto.Parcela{Codigo: n, Fracao: p.Fracao})
			}
			sort.SliceStable(porNome, func(i, j int) bool { return porNome[i].Fracao > porNome[j].Fracao })
			var ps []string
			resto := 1.0
			for _, p := range porNome[:min(3, len(porNome))] {
				if p.Fracao < 0.01 {
					break
				}
				ps = append(ps, fmt.Sprintf("%s %s", p.Codigo, Percent(p.Fracao)))
				resto -= p.Fracao
			}
			if resto > 0.005 {
				ps = append(ps, "outras "+Percent(resto))
			}
			fmt.Fprintf(w, "    cargas: %s.\n", strings.Join(ps, ", "))
		}
	}
}

// WritePortoCenarios escreve a tabela de decisão de um grupo. cs[0] é a
// referência (o modelo sem mudança); real é o que o dado mostra.
func WritePortoCenarios(w io.Writer, c *porto.Config, grupo string, real porto.Resumo, cs []porto.Comparacao) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(w, "%s — %s\n", grupo, periodoLabel(c))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	custo := ""
	if c.CustoNavioDia > 0 {
		custo = "\tcusto evitado (US$)"
	}
	fmt.Fprintf(tw, "  cenário\tespera mediana\tespera média\tnavio-dias\tvariação\tocupação%s\n", custo)
	vazio := ""
	if custo != "" {
		vazio = "\t—"
	}
	fmt.Fprintf(tw, "  real (dado)\t%s\t%s\t%s\t—\t%s%s\n", Dias(real.EsperaMediana), Dias(real.EsperaMedia),
		Milhar(real.NavioDias), Percent(real.Ocupacao), vazio)
	ref := cs[0]
	for k, x := range cs {
		mlo, mhi := x.Faixa(func(r porto.Resumo) float64 { return r.EsperaMediana })
		alo, ahi := x.Faixa(func(r porto.Resumo) float64 { return r.EsperaMedia })
		nlo, nhi := x.Faixa(func(r porto.Resumo) float64 { return r.NavioDias })
		olo, ohi := x.Faixa(func(r porto.Resumo) float64 { return r.Ocupacao })
		delta, extra := "—", vazio
		if k > 0 {
			delta = faixaPercent(x.Variacao(ref, func(r porto.Resumo) float64 { return r.NavioDias }))
			if c.CustoNavioDia > 0 {
				a := (ref.Otimista.NavioDias - x.Otimista.NavioDias) * c.CustoNavioDia
				b := (ref.Conservadora.NavioDias - x.Conservadora.NavioDias) * c.CustoNavioDia
				extra = "\t" + faixaMilhar(min(a, b), max(a, b))
			}
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s%s\n", x.Cenario.Nome, FaixaDias(mlo, mhi), FaixaDias(alo, ahi),
			faixaMilhar(nlo, nhi), delta, faixaPercent(olo, ohi), extra)
	}
	tw.Flush()
}

// WritePortoCapacidade escreve quantos navios por mês cabem na meta.
func WritePortoCapacidade(w io.Writer, grupo string, cs []porto.Capacidade) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(w, "%s — hoje: %s navios/mês, espera mediana real de %s. Meta: até %s.\n", grupo,
		decimal(cs[0].HojeMes, 1), Dias(cs[0].HojeMediana), Dias(cs[0].MetaHoras))
	cabe := func(x, hoje float64) string {
		if x == 0 {
			return "não atinge a meta"
		}
		d := percentLimpo(x/hoje - 1)
		switch {
		case d == "0%":
			d = "igual a hoje"
		case x > hoje:
			d = "+" + d
		}
		return fmt.Sprintf("%s (%s)", decimal(x, 1), d)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  cenário\tcabem (otimista)\tcabem (conservadora)\n")
	semMeta := false
	for _, x := range cs {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", x.Cenario.Nome, cabe(x.Otimista, x.HojeMes), cabe(x.Conservador, x.HojeMes))
		semMeta = semMeta || x.Conservador == 0 || x.Otimista == 0
	}
	tw.Flush()
	if semMeta {
		fmt.Fprintf(w, "  \"não atinge a meta\": nem com %s da demanda de hoje. Parte da espera não depende de berço\n", Percent(porto.Niveis[0]))
		fmt.Fprintln(w, "  (o navio esperava a vez da própria carga), e menos navios não a elimina.")
	}

	// a curva do primeiro cenário: a espera explode perto do limite
	fmt.Fprintf(w, "\n  espera mediana conforme a demanda (%s):\n", cs[0].Cenario.Nome)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "  navios/mês\tespera mediana\t\n")
	for _, p := range cs[0].Curva {
		f := p.NaviosMes / cs[0].HojeMes
		if math.Abs(math.Mod(f*100+0.5, 25)-0.5) > 0.6 { // de 25 em 25%
			continue
		}
		marca := ""
		if math.Abs(f-1) < 0.01 {
			marca = "  ← hoje"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", decimal(p.NaviosMes, 1),
			FaixaDias(min(p.MedianaOtimista, p.MedianaCons), max(p.MedianaOtimista, p.MedianaCons)), marca)
	}
	tw.Flush()
}

// WritePortoPrevisao escreve o erro de cada método de previsão.
// escolhido é o método eleito no treino.
func WritePortoPrevisao(w io.Writer, grupo string, espera porto.Resumo, as []porto.Avaliacao, escolhido porto.Metodo) {
	fmt.Fprintf(w, "%s — %d navios; espera real: mediana %s, média %s\n", grupo, espera.Navios, Dias(espera.EsperaMediana), Dias(espera.EsperaMedia))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  método\terro médio\terro mediano\tviés\tacerta em ±2 dias\n")
	for _, a := range as {
		m := ""
		if a.Metodo == escolhido {
			m = "  ← escolhido no treino"
		}
		vies := Dias(a.Vies)
		if math.Round(a.Vies) > 0 {
			vies = "+" + vies
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s%s\n", a.Metodo, Dias(a.ErroMedio), Dias(a.ErroMediano), vies, Percent(a.Dentro48h), m)
	}
	tw.Flush()
}

// WritePortoChegada escreve o efeito da chegada just-in-time num grupo.
func WritePortoChegada(w io.Writer, grupo string, rs []porto.ResultadoChegada) {
	fmt.Fprintf(w, "%s\n", grupo)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  política\tnavio-dias fundeado\tevitado\tatracação atrasou (média)\tnavios atrasados >6 h\n")
	for i, r := range rs {
		ev, at, na := "—", "—", "—"
		if i > 0 {
			ev, at, na = percentLimpo(r.Economia), Dias(r.AtrasoMedio), Percent(r.Atrasados)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", r.Politica.Nome, Milhar(r.FundeadoDias), ev, at, na)
	}
	tw.Flush()
}

func plural(n int, um, varios string) string {
	if n == 1 {
		return "1 " + um
	}
	return fmt.Sprintf("%d %s", n, varios)
}

// percentLimpo é Percent com sinal de menos e sem "-0%".
func percentLimpo(x float64) string {
	if math.Round(x*100) == 0 {
		return "0%"
	}
	return strings.Replace(Percent(x), "-", "−", 1)
}
