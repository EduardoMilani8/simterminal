// Package porto modela a fila de navios de um porto a partir das
// atracações reais da ANTAQ.
//
// Nada de distribuição inventada: as chegadas, a ordem em que os navios
// atracaram e o tempo de cada um no berço vêm do dado. O modelo só troca
// o que o cenário manda trocar — um berço a mais, operação mais rápida,
// navio chegando na hora certa — e o resto do ano acontece como
// aconteceu. Sem mudança nenhuma, ele reproduz o ano real; é essa a
// âncora (TestReplayReproduzOReal).
package porto

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config é o arquivo de um porto.
type Config struct {
	Porto string `yaml:"porto"` // como a ANTAQ grafa "Porto Atracação"
	Dados string `yaml:"dados"` // pasta com os arquivos da ANTAQ
	Anos  []int  `yaml:"anos"`  // anos a carregar (precisa cobrir treino e período, com folga)

	Periodo Janela `yaml:"periodo"` // o que se analisa
	Treino  Janela `yaml:"treino"`  // de onde a previsão aprende; antes do período

	// CustoNavioDia converte navio-dia de espera em dinheiro. Opcional: o
	// valor muda muito com o tipo de navio e o frete do momento, então
	// fica a cargo de quem usa. Zero omite as colunas de custo.
	CustoNavioDia float64 `yaml:"custo_navio_dia_usd"`

	Grupos   []Grupo   `yaml:"grupos"`
	Cenarios []Cenario `yaml:"cenarios"`

	dir string // pasta do arquivo, para resolver Dados relativo
}

// Janela é um intervalo [Inicio, Fim) de datas.
type Janela struct {
	Inicio Data `yaml:"inicio"`
	Fim    Data `yaml:"fim"`
}

// Contem diz se t está na janela.
func (j Janela) Contem(t time.Time) bool {
	return !t.Before(j.Inicio.Time) && t.Before(j.Fim.Time)
}

// Data é uma data aaaa-mm-dd no YAML.
type Data struct{ time.Time }

func (d *Data) UnmarshalYAML(n *yaml.Node) error {
	t, err := time.Parse("2006-01-02", n.Value)
	if err != nil {
		return fmt.Errorf("linha %d: data %q, use aaaa-mm-dd", n.Line, n.Value)
	}
	d.Time = t
	return nil
}

// Grupo é um conjunto de berços que disputam os mesmos navios: o corredor
// de exportação de grãos, os berços de fertilizante. Quem define é quem
// conhece o porto; `simterminal porto bercos` mostra a carga de cada
// berço para ajudar.
type Grupo struct {
	Nome   string   `yaml:"nome"`
	Bercos []string `yaml:"bercos"` // IDBerco da ANTAQ
}

// Cenario é uma mudança a testar. Campos zerados não mudam nada.
type Cenario struct {
	Nome  string `yaml:"nome"`
	Grupo string `yaml:"grupo"` // vazio: vale para todos os grupos

	// Alocacao "por_berco" (padrão): cada navio no berço em que atracou de
	// fato, e o berço atende o primeiro pronto da sua fila, na ordem do
	// line-up real. "compartilhada": fila única, qualquer berço livre do
	// grupo serve. "fixa": cada berço repete a sequência real e espera o
	// próximo dela mesmo que outro esteja pronto — reproduz o ano exato,
	// mas só vale para mudanças pequenas: se os tempos mudam muito, o
	// berço fica esperando um navio que na nova linha do tempo chega bem
	// depois dos outros.
	Alocacao string `yaml:"alocacao"`
	// Ordem na fila compartilhada: "real" (a ordem em que atracaram, que
	// reflete o line-up do porto) ou "chegada" (FIFO). Padrão: real.
	Ordem string `yaml:"ordem"`

	BercosExtra int `yaml:"bercos_extra"` // exige alocação compartilhada

	// Multiplicadores sobre o tempo no berço. 0.9 em Operacao = operar
	// 10% mais rápido. Ocioso é o tempo atracado sem operar (antes de
	// começar e depois de terminar: T2 e T4 da ANTAQ).
	Operacao *float64 `yaml:"operacao"`
	Ocioso   *float64 `yaml:"ocioso"`

	// Demanda multiplica o ritmo de chegada: 1.2 = 20% mais navios no
	// mesmo tempo.
	Demanda *float64 `yaml:"demanda"`
}

// Fatores devolve os multiplicadores de operação, ocioso e demanda, com
// 1 para os que não foram informados.
func (s *Cenario) Fatores() (operacao, ocioso, demanda float64) {
	f := func(p *float64) float64 {
		if p == nil {
			return 1
		}
		return *p
	}
	return f(s.Operacao), f(s.Ocioso), f(s.Demanda)
}

// Fator devolve um ponteiro para x, para montar cenários em código.
func Fator(x float64) *float64 { return &x }

// Carregar lê e valida o arquivo do porto.
func Carregar(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c.dir = filepath.Dir(path)
	if err := c.validar(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// PastaDados devolve a pasta dos arquivos da ANTAQ, relativa ao YAML.
func (c *Config) PastaDados() string {
	if filepath.IsAbs(c.Dados) || c.dir == "" {
		return c.Dados
	}
	return filepath.Join(c.dir, c.Dados)
}

func (c *Config) validar() error {
	if c.Porto == "" {
		return fmt.Errorf("falta o campo porto")
	}
	if c.Dados == "" {
		c.Dados = "dados/antaq"
	}
	if len(c.Anos) == 0 {
		return fmt.Errorf("falta a lista de anos")
	}
	if c.Periodo.Inicio.IsZero() || !c.Periodo.Fim.After(c.Periodo.Inicio.Time) {
		return fmt.Errorf("periodo: informe inicio e fim, com fim depois do inicio")
	}
	if !c.Treino.Inicio.IsZero() && c.Treino.Fim.After(c.Periodo.Inicio.Time) {
		return fmt.Errorf("treino precisa terminar antes do periodo começar, senão a previsão cola a resposta")
	}
	if len(c.Grupos) == 0 {
		return fmt.Errorf("defina pelo menos um grupo de berços")
	}
	nomes := map[string]bool{}
	bercos := map[string]string{}
	for _, g := range c.Grupos {
		if g.Nome == "" || len(g.Bercos) == 0 {
			return fmt.Errorf("grupo sem nome ou sem berços")
		}
		if nomes[g.Nome] {
			return fmt.Errorf("grupo %q repetido", g.Nome)
		}
		nomes[g.Nome] = true
		for _, b := range g.Bercos {
			if outro, ok := bercos[b]; ok {
				return fmt.Errorf("berço %s está em %q e em %q", b, outro, g.Nome)
			}
			bercos[b] = g.Nome
		}
	}
	for i := range c.Cenarios {
		if err := c.Cenarios[i].validar(nomes); err != nil {
			return fmt.Errorf("cenário %q: %w", c.Cenarios[i].Nome, err)
		}
	}
	return nil
}

func (s *Cenario) validar(grupos map[string]bool) error {
	if s.Nome == "" {
		return fmt.Errorf("cenário sem nome")
	}
	if s.Grupo != "" && !grupos[s.Grupo] {
		return fmt.Errorf("grupo %q não existe", s.Grupo)
	}
	switch s.Alocacao {
	case "":
		s.Alocacao = AlocPorBerco
	case AlocFixa, AlocPorBerco, AlocCompartilhada:
	default:
		return fmt.Errorf("alocacao %q: use fixa, por_berco ou compartilhada", s.Alocacao)
	}
	switch s.Ordem {
	case "":
		s.Ordem = OrdemReal
	case OrdemReal, OrdemChegada:
	default:
		return fmt.Errorf("ordem %q: use real ou chegada", s.Ordem)
	}
	if s.Ordem == OrdemChegada && s.Alocacao == AlocFixa {
		return fmt.Errorf("ordem chegada não vale com alocacao fixa, que segue a sequência real")
	}
	if s.BercosExtra < 0 {
		return fmt.Errorf("bercos_extra negativo")
	}
	if s.BercosExtra > 0 && s.Alocacao != AlocCompartilhada {
		return fmt.Errorf("bercos_extra exige alocacao compartilhada: com alocação fixa nenhum navio iria para o berço novo")
	}
	for nome, v := range map[string]*float64{"operacao": s.Operacao, "ocioso": s.Ocioso, "demanda": s.Demanda} {
		if v != nil && *v < 0 {
			return fmt.Errorf("%s negativo", nome)
		}
	}
	if s.Operacao != nil && *s.Operacao == 0 {
		return fmt.Errorf("operacao 0 significaria operar em tempo zero")
	}
	if s.Demanda != nil && *s.Demanda == 0 {
		return fmt.Errorf("demanda 0 significaria nenhum navio")
	}
	return nil
}

// Valores de Alocacao e Ordem.
const (
	AlocFixa          = "fixa"
	AlocPorBerco      = "por_berco"
	AlocCompartilhada = "compartilhada"
	OrdemReal         = "real"
	OrdemChegada      = "chegada"
)

// Base é o replay sem mudança na sequência real: reproduz o que
// aconteceu, navio a navio.
func Base() Cenario {
	return Cenario{Nome: "real", Alocacao: AlocFixa, Ordem: OrdemReal}
}

// SemMudanca é a referência dos cenários: o modelo por berço, sem mudar
// nada. Fica perto do real (na leitura conservadora, a poucos por cento)
// mas não igual, porque o berço não espera quem não chegou.
func SemMudanca() Cenario {
	return Cenario{Nome: "modelo, sem mudança", Alocacao: AlocPorBerco, Ordem: OrdemReal}
}
