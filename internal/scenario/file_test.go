package scenario

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/eduardomilani8/simterminal/internal/terminal"
)

const mapYAML = `
nome: atual
horizonte_min: 720
replicas: 30
warmup_min: 60
chegadas:
  perfil_horario: [4, 12, 28, 31, 22, 15, 9, 6, 6, 8, 5, 3]
recursos:
  balanca_entrada: 1
  balanca_saida: 1
  docas: 12
descarga:
  granel:     {min: 35, moda: 48, max: 95}
  paletizada: {min: 18, moda: 25, max: 40}
`

func TestParseMapExample(t *testing.T) {
	s, err := Parse([]byte(mapYAML))
	if err != nil {
		t.Fatal(err)
	}
	c := s.Config
	if s.Name != "atual" || s.Replicas != 30 || s.Seed != DefaultSeed {
		t.Errorf("cabeçalho: %+v", s)
	}
	if c.Horizon != 720 || c.Warmup != 60 || len(c.HourlyProfile) != 12 {
		t.Errorf("tempo: %+v", c)
	}
	if c.EntryScales != 1 || c.ExitScales != 1 || c.Docks != 12 {
		t.Errorf("recursos: %+v", c)
	}
	if c.Weighing != DefaultWeighing {
		t.Errorf("pesagem padrão não aplicada: %+v", c.Weighing)
	}
	want := []terminal.Cargo{
		{Name: "granel", Unload: terminal.Triangular{Min: 35, Mode: 48, Max: 95}, Share: 0.5},
		{Name: "paletizada", Unload: terminal.Triangular{Min: 18, Mode: 25, Max: 40}, Share: 0.5},
	}
	for i := range want {
		if c.Cargo[i] != want[i] {
			t.Errorf("carga %d = %+v, esperava %+v", i, c.Cargo[i], want[i])
		}
	}
}

func TestParseOptionalFields(t *testing.T) {
	s, err := Parse([]byte(`
chegadas:
  perfil_horario: [10, 10]
  agendados: 0.5
  tolerancia_min: 5
recursos: {balanca_entrada: 2, balanca_saida: 0, docas: 4}
pesagem: {min: 2, moda: 3, max: 5}
semente: 99
descarga:
  granel: {min: 30, moda: 40, max: 60, proporcao: 0.25}
  saca:   {min: 20, moda: 25, max: 30, proporcao: 0.75}
`))
	if err != nil {
		t.Fatal(err)
	}
	c := s.Config
	if c.Horizon != 120 {
		t.Errorf("sem horizonte, deveria cobrir o perfil: %g", c.Horizon)
	}
	if c.ExitScales != 0 || c.EntryScales != 2 || s.Seed != 99 || c.Scheduled != 0.5 || c.ScheduleTolerance != 5 {
		t.Errorf("campos opcionais: %+v", s)
	}
	if c.Cargo[0].Share != 0.25 || c.Cargo[1].Name != "saca" {
		t.Errorf("mix: %+v", c.Cargo)
	}
}

func TestUnknownFieldMessage(t *testing.T) {
	_, err := Parse([]byte(strings.Replace(mapYAML, "docas: 12", "docaz: 12", 1)))
	if err == nil || !strings.Contains(err.Error(), `campo desconhecido "docaz"`) || strings.Contains(err.Error(), "struct") {
		t.Errorf("mensagem ruim: %v", err)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"campo com erro de digitação": strings.Replace(mapYAML, "balanca_entrada", "balanca_entrda", 1),
		"uma réplica só":              strings.Replace(mapYAML, "replicas: 30", "replicas: 1", 1),
		"sem docas":                   strings.Replace(mapYAML, "docas: 12", "docas: 0", 1),
		"proporção só num tipo":       strings.Replace(mapYAML, "max: 95}", "max: 95, proporcao: 0.3}", 1),
		"triangular invertida":        strings.Replace(mapYAML, "min: 35, moda: 48", "min: 50, moda: 48", 1),
		"vazio":                       "",
	}
	for name, doc := range cases {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: deveria dar erro", name)
		}
	}
}

// TestExampleScenariosLoad garante que os exemplos do repositório
// continuam válidos.
func TestExampleScenariosLoad(t *testing.T) {
	files, err := filepath.Glob("../../exemplos/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("nenhum exemplo encontrado (%v)", err)
	}
	for _, f := range files {
		if _, err := Load(f); err != nil {
			t.Errorf("%v", err)
		}
	}
}
