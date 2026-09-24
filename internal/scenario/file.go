package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/eduardomilani8/simterminal/internal/terminal"
)

// Valores usados quando o arquivo não informa.
const (
	DefaultReplicas = 30
	DefaultSeed     = 1
)

// DefaultWeighing é o tempo de pesagem quando o arquivo não informa: a
// balança de "uns quatro minutos".
var DefaultWeighing = terminal.Triangular{Min: 3, Mode: 4, Max: 7}

// Scenario é um cenário lido de arquivo, pronto para rodar.
type Scenario struct {
	Name     string
	Config   terminal.Config
	Replicas int
	Seed     int64
}

// arquivo espelha o YAML. Campos em português, como o operador escreve.
type arquivo struct {
	Nome         string  `yaml:"nome"`
	HorizonteMin float64 `yaml:"horizonte_min"`
	Replicas     *int    `yaml:"replicas"`
	Semente      *int64  `yaml:"semente"`
	WarmupMin    float64 `yaml:"warmup_min"`
	Chegadas     struct {
		PerfilHorario []float64 `yaml:"perfil_horario"`
		Agendados     float64   `yaml:"agendados"`
		ToleranciaMin *float64  `yaml:"tolerancia_min"`
	} `yaml:"chegadas"`
	Recursos struct {
		BalancaEntrada *int `yaml:"balanca_entrada"`
		BalancaSaida   *int `yaml:"balanca_saida"`
		Docas          int  `yaml:"docas"`
	} `yaml:"recursos"`
	Pesagem  *terminal.Triangular `yaml:"pesagem"`
	Descarga map[string]descarga  `yaml:"descarga"`
}

type descarga struct {
	terminal.Triangular `yaml:",inline"`
	Proporcao           *float64 `yaml:"proporcao"`
}

// Load lê um cenário de um arquivo YAML.
func Load(path string) (Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	s, err := Parse(data)
	if err != nil {
		return Scenario{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.Name == "" {
		s.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return s, nil
}

// Parse interpreta o conteúdo de um arquivo de cenário. Campo
// desconhecido é erro: um "balanca_entrda" silenciosamente ignorado vira
// uma tarde perdida.
func Parse(data []byte) (Scenario, error) {
	var a arquivo
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&a); err != nil {
		if errors.Is(err, io.EOF) {
			return Scenario{}, errors.New("arquivo vazio")
		}
		return Scenario{}, friendlyYAMLError(err)
	}

	s := Scenario{Name: a.Nome, Replicas: DefaultReplicas, Seed: DefaultSeed}
	if a.Replicas != nil {
		s.Replicas = *a.Replicas
	}
	if a.Semente != nil {
		s.Seed = *a.Semente
	}
	if s.Replicas < 2 {
		return Scenario{}, fmt.Errorf("replicas = %d: precisa de pelo menos 2 para ter intervalo de confiança", s.Replicas)
	}

	c := terminal.Config{
		Horizon:           a.HorizonteMin,
		Warmup:            a.WarmupMin,
		HourlyProfile:     a.Chegadas.PerfilHorario,
		EntryScales:       1,
		ExitScales:        1,
		Docks:             a.Recursos.Docas,
		Weighing:          DefaultWeighing,
		Scheduled:         a.Chegadas.Agendados,
		ScheduleTolerance: 15,
	}
	if c.Horizon == 0 {
		c.Horizon = 60 * float64(len(c.HourlyProfile))
	}
	if a.Recursos.BalancaEntrada != nil {
		c.EntryScales = *a.Recursos.BalancaEntrada
	}
	if a.Recursos.BalancaSaida != nil {
		c.ExitScales = *a.Recursos.BalancaSaida
	}
	if a.Pesagem != nil {
		c.Weighing = *a.Pesagem
	}
	if a.Chegadas.ToleranciaMin != nil {
		c.ScheduleTolerance = *a.Chegadas.ToleranciaMin
	}

	cargo, err := cargoMix(a.Descarga)
	if err != nil {
		return Scenario{}, err
	}
	c.Cargo = cargo

	if err := c.Validate(); err != nil {
		return Scenario{}, err
	}
	s.Config = c
	return s, nil
}

var unknownField = regexp.MustCompile(`field (\S+) not found in type .*`)

// friendlyYAMLError troca o "field x not found in type struct {...}" do
// yaml por algo que quem escreveu o arquivo entenda.
func friendlyYAMLError(err error) error {
	var te *yaml.TypeError
	if !errors.As(err, &te) {
		return err
	}
	msgs := make([]string, len(te.Errors))
	for i, m := range te.Errors {
		msgs[i] = unknownField.ReplaceAllString(m, "campo desconhecido \"$1\"")
	}
	return errors.New(strings.Join(msgs, "; "))
}

// cargoMix monta os tipos de carga em ordem alfabética — a ordem entra no
// sorteio do tipo, então precisa ser estável. Sem proporção em nenhum,
// divide igualmente.
func cargoMix(d map[string]descarga) ([]terminal.Cargo, error) {
	names := make([]string, 0, len(d))
	withShare := 0
	for name, v := range d {
		names = append(names, name)
		if v.Proporcao != nil {
			withShare++
		}
	}
	sort.Strings(names)
	if withShare != 0 && withShare != len(names) {
		return nil, errors.New("descarga: informe a proporção em todos os tipos de carga ou em nenhum")
	}

	cargo := make([]terminal.Cargo, 0, len(names))
	for _, name := range names {
		v := d[name]
		share := 1 / float64(len(names))
		if v.Proporcao != nil {
			share = *v.Proporcao
		}
		cargo = append(cargo, terminal.Cargo{Name: name, Unload: v.Triangular, Share: share})
	}
	return cargo, nil
}
