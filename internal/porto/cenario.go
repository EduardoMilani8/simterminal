package porto

// Comparacao é um cenário rodado num grupo, nas duas leituras de
// prontidão. O efeito verdadeiro fica entre as duas.
type Comparacao struct {
	Cenario      Cenario
	Otimista     Resumo
	Conservadora Resumo
}

// Faixa devolve o menor e o maior valor de f entre as duas leituras.
func (c Comparacao) Faixa(f func(Resumo) float64) (lo, hi float64) {
	a, b := f(c.Otimista), f(c.Conservadora)
	return min(a, b), max(a, b)
}

// Variacao devolve a faixa da variação de f em relação à referência,
// comparando cada leitura de prontidão com a mesma leitura da
// referência.
func (c Comparacao) Variacao(ref Comparacao, f func(Resumo) float64) (lo, hi float64) {
	v := func(a, b Resumo) float64 {
		if f(b) == 0 {
			return 0
		}
		return f(a)/f(b) - 1
	}
	x, y := v(c.Otimista, ref.Otimista), v(c.Conservadora, ref.Conservadora)
	return min(x, y), max(x, y)
}

// Comparar roda os cenários que valem para o grupo. O primeiro item é
// sempre a referência: o modelo sem mudança.
func Comparar(d *DadosGrupo, cenarios []Cenario) []Comparacao {
	todos := append([]Cenario{SemMudanca()}, cenarios...)
	var out []Comparacao
	for _, sc := range todos {
		if sc.Grupo != "" && sc.Grupo != d.Grupo.Nome {
			continue
		}
		out = append(out, Comparacao{
			Cenario:      sc,
			Otimista:     Replay{Dados: d, Cenario: sc, Modo: Otimista}.Rodar().Resumo(),
			Conservadora: Replay{Dados: d, Cenario: sc, Modo: Conservadora}.Rodar().Resumo(),
		})
	}
	return out
}
