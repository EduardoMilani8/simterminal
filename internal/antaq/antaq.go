// Package antaq lê os dados abertos do Estatístico Aquaviário da ANTAQ:
// uma linha por atracação de navio em porto brasileiro, com os instantes
// de chegada, atracação, operação e desatracação.
//
// Os arquivos são os mesmos que qualquer pessoa baixa do site da ANTAQ
// ({ano}Atracacao.txt, {ano}TemposAtracacao.txt, {ano}Carga.txt): texto
// UTF-8 com BOM, separado por ponto e vírgula, datas dd/mm/aaaa e vírgula
// decimal. Nada aqui é inventado; o que o arquivo não diz fica como
// ausente.
package antaq

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Atracacao é a passagem de um navio por um berço.
type Atracacao struct {
	ID        string
	Porto     string // "Porto Atracação", ex.: "Paranaguá"
	CDTUP     string // código do porto, ex.: "BRPNG"
	Berco     string // IDBerco, ex.: "PNG0212"
	NomeBerco string
	Terminal  string

	Chegada      time.Time // chegada à área de fundeio
	Atracacao    time.Time
	InicioOp     time.Time // zero se ausente
	FimOp        time.Time // zero se ausente
	Desatracacao time.Time

	TipoOperacao string // "Movimentação da Carga", "Apoio", ...
	Navegacao    string // "Longo Curso", "Cabotagem", ...
	IMO          string

	// Tempos calculados pela própria ANTAQ, em horas. NaN quando a ANTAQ
	// marcou o valor como discrepante ou quando não veio.
	TEspera, TEsperaInicioOp, TOperacao, TEsperaDesatracacao, TAtracado float64

	// Carga principal (a de maior peso). Vazia se {ano}Carga.txt não foi
	// carregado.
	Mercadoria string // código NCM SH4, ex.: "1201"
	Natureza   string // "Granel Sólido", "Carga Conteinerizada", ...
	Sentido    string // "Embarcados" ou "Desembarcados"
	Toneladas  float64
}

// Completa diz se a atracação tem os três instantes que o modelo de fila
// precisa, em ordem.
func (a *Atracacao) Completa() bool {
	return !a.Chegada.IsZero() && !a.Atracacao.IsZero() && !a.Desatracacao.IsZero() &&
		!a.Atracacao.Before(a.Chegada) && a.Desatracacao.After(a.Atracacao)
}

// Espera devolve as horas entre a chegada e a atracação.
func (a *Atracacao) Espera() float64 { return horas(a.Atracacao.Sub(a.Chegada)) }

// NoBerco devolve as horas em que o navio ocupou o berço.
func (a *Atracacao) NoBerco() float64 { return horas(a.Desatracacao.Sub(a.Atracacao)) }

func horas(d time.Duration) float64 { return d.Hours() }

// Filtro decide quais atracações manter. Recebe a linha já lida, sem a
// carga.
type Filtro func(*Atracacao) bool

// DoPorto mantém as atracações do porto dado, pelo nome como a ANTAQ
// grafa em "Porto Atracação". Todas, não só as de carga: abastecimento e
// reparo também ocupam berço.
func DoPorto(nome string) Filtro {
	return func(a *Atracacao) bool { return a.Porto == nome }
}

// Carregar lê os anos pedidos de dir. Cada ano precisa de
// {ano}Atracacao.txt e {ano}TemposAtracacao.txt. Com carga, lê também
// {ano}Carga.txt quando existir — são 400 MB por ano, então só quando a
// mercadoria importa. Uma mesma atracação que apareça em dois anos é
// contada uma vez. O resultado sai ordenado por atracação.
func Carregar(dir string, anos []int, f Filtro, carga bool) ([]Atracacao, error) {
	var todas []Atracacao
	vistos := map[string]bool{}
	for _, ano := range anos {
		as, err := lerAtracacoes(filepath.Join(dir, fmt.Sprintf("%dAtracacao.txt", ano)), f)
		if err != nil {
			return nil, err
		}
		idx := map[string]int{}
		for i := range as {
			idx[as[i].ID] = i
		}
		if err := lerTempos(filepath.Join(dir, fmt.Sprintf("%dTemposAtracacao.txt", ano)), as, idx); err != nil {
			return nil, err
		}
		cargaPath := filepath.Join(dir, fmt.Sprintf("%dCarga.txt", ano))
		if _, err := os.Stat(cargaPath); err == nil && carga {
			if err := lerCarga(cargaPath, as, idx); err != nil {
				return nil, err
			}
		}
		for _, a := range as {
			if !vistos[a.ID] {
				vistos[a.ID] = true
				todas = append(todas, a)
			}
		}
	}
	sort.SliceStable(todas, func(i, j int) bool { return todas[i].Atracacao.Before(todas[j].Atracacao) })
	return todas, nil
}

func lerAtracacoes(path string, f Filtro) ([]Atracacao, error) {
	var out []Atracacao
	err := lerTabela(path, []string{
		"IDAtracacao", "CDTUP", "IDBerco", "Berço", "Porto Atracação", "Terminal",
		"Data Chegada", "Data Atracação", "Data Início Operação", "Data Término Operação",
		"Data Desatracação", "Tipo de Operação", "Tipo de Navegação da Atracação", "Nº do IMO",
	}, func(v []string) error {
		a := Atracacao{
			ID: v[0], CDTUP: v[1], Berco: v[2], NomeBerco: v[3], Porto: v[4], Terminal: v[5],
			TipoOperacao: v[11], Navegacao: v[12], IMO: v[13],
			TEspera: math.NaN(), TEsperaInicioOp: math.NaN(), TOperacao: math.NaN(),
			TEsperaDesatracacao: math.NaN(), TAtracado: math.NaN(),
		}
		if f != nil && !f(&a) {
			return nil
		}
		var err error
		for i, dst := range []*time.Time{&a.Chegada, &a.Atracacao, &a.InicioOp, &a.FimOp, &a.Desatracacao} {
			if *dst, err = data(v[6+i]); err != nil {
				return fmt.Errorf("atracação %s: %w", a.ID, err)
			}
		}
		out = append(out, a)
		return nil
	})
	return out, err
}

func lerTempos(path string, as []Atracacao, idx map[string]int) error {
	return lerTabela(path, []string{
		"IDAtracacao", "TEsperaAtracacao", "TEsperaInicioOp", "TOperacao", "TEsperaDesatracacao", "TAtracado",
	}, func(v []string) error {
		i, ok := idx[v[0]]
		if !ok {
			return nil
		}
		a := &as[i]
		for k, dst := range []*float64{&a.TEspera, &a.TEsperaInicioOp, &a.TOperacao, &a.TEsperaDesatracacao, &a.TAtracado} {
			*dst = numero(v[1+k])
		}
		return nil
	})
}

// lerCarga guarda em cada atracação a mercadoria de maior peso entre as
// que contam como movimentação (FlagMCOperacaoCarga = 1) e o peso total.
func lerCarga(path string, as []Atracacao, idx map[string]int) error {
	maior := map[int]float64{}
	return lerTabela(path, []string{
		"IDAtracacao", "CDMercadoria", "Natureza da Carga", "Sentido", "VLPesoCargaBruta", "FlagMCOperacaoCarga",
	}, func(v []string) error {
		i, ok := idx[v[0]]
		if !ok || v[5] != "1" {
			return nil
		}
		peso := numero(v[4])
		if math.IsNaN(peso) {
			return nil
		}
		a := &as[i]
		a.Toneladas += peso
		if m, ok := maior[i]; !ok || peso > m {
			maior[i] = peso
			a.Mercadoria, a.Natureza, a.Sentido = v[1], v[2], v[3]
		}
		return nil
	})
}
