package porto

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eduardomilani8/simterminal/internal/antaq"
)

// Uma sobreposição de minutos (erro de registro) não pode virar mais uma
// posição no berço, e o navio "por cima" não pode travar a fila.
func TestSobreposicaoCurtaNaoViraPosicao(t *testing.T) {
	as := []antaq.Atracacao{
		atr("1", "B", 0, 1, 100),
		atr("2", "B", 50, 99.5, 150), // atraca meia hora antes do 1 sair
		atr("3", "B", 60, 152, 200),
	}
	d := MontarGrupo(Grupo{Nome: "g", Bercos: []string{"B"}}, as, periodo(300))
	if d.Posicoes["B"] != 1 {
		t.Fatalf("posições = %d, esperava 1", d.Posicoes["B"])
	}
	if !d.Navios[1].Excedente || d.Navios[0].Excedente || d.Navios[2].Excedente {
		t.Fatalf("só o navio 2 deveria ser excedente: %v %v %v", d.Navios[0].Excedente, d.Navios[1].Excedente, d.Navios[2].Excedente)
	}
	for _, sc := range []Cenario{Base(), {Nome: "b", Alocacao: AlocPorBerco, Ordem: OrdemReal}} {
		r := Replay{Dados: d, Cenario: sc}.Rodar()
		for i, n := range d.Navios {
			if math.Abs(r.Atracou[i]-n.Atracacao) > 1e-9 {
				t.Errorf("%s: navio %s atracou %.2f, real %.2f", sc.Alocacao, n.ID, r.Atracou[i], n.Atracacao)
			}
		}
	}
}

// Invariantes que valem em qualquer cenário: ninguém atraca antes de
// chegar, ninguém sai antes de atracar, e cada posição recebe um navio
// por vez.
func TestInvariantesDoReplay(t *testing.T) {
	cenarios := []Cenario{
		Base(),
		{Nome: "por berço", Alocacao: AlocPorBerco, Ordem: OrdemReal},
		{Nome: "compartilhada", Alocacao: AlocCompartilhada, Ordem: OrdemReal},
		{Nome: "fifo", Alocacao: AlocCompartilhada, Ordem: OrdemChegada},
		{Nome: "+2", Alocacao: AlocCompartilhada, Ordem: OrdemReal, BercosExtra: 2},
		{Nome: "rápida", Alocacao: AlocFixa, Ordem: OrdemReal, Operacao: Fator(0.5), Ocioso: Fator(0)},
		{Nome: "demanda", Alocacao: AlocFixa, Ordem: OrdemReal, Demanda: Fator(1.5)},
	}
	chamadas := []*Chamada{nil, {Reserva: 1, Antecedencia: 48}, {Antecedencia: 24}}
	for _, d := range append(paranagua(t), grupoPequeno()) {
		for _, sc := range cenarios {
			for _, ch := range chamadas {
				if ch != nil && sc.Alocacao == AlocFixa {
					continue
				}
				for _, modo := range []Prontidao{Otimista, Conservadora} {
					r := Replay{Dados: d, Cenario: sc, Modo: modo, Chamada: ch}.Rodar()
					for i, n := range d.Navios {
						if !r.Simulado(i) {
							continue
						}
						if r.Atracou[i] < r.Chegada[i]-1e-9 {
							t.Fatalf("%s/%s/%s: navio %s atracou %.1f antes de chegar %.1f", d.Grupo.Nome, sc.Nome, modo, n.ID, r.Atracou[i], r.Chegada[i])
						}
						if r.Saiu[i] < r.Atracou[i] {
							t.Fatalf("%s/%s: navio %s saiu antes de atracar", d.Grupo.Nome, sc.Nome, n.ID)
						}
					}
					if max := maxSimultaneos(d, r); max > r.Posicoes+contarExcedentes(d) {
						t.Fatalf("%s/%s/%s: %d navios no berço ao mesmo tempo, com %d posições", d.Grupo.Nome, sc.Nome, modo, max, r.Posicoes)
					}
				}
			}
		}
	}
}

func maxSimultaneos(d *DadosGrupo, r Resultado) int {
	type ev struct {
		t float64
		k int
	}
	var evs []ev
	for i := range d.Navios {
		evs = append(evs, ev{r.Atracou[i], 1}, ev{r.Saiu[i], -1})
	}
	// saída antes de entrada no mesmo instante
	sort.Slice(evs, func(i, j int) bool {
		if evs[i].t == evs[j].t {
			return evs[i].k < evs[j].k
		}
		return evs[i].t < evs[j].t
	})
	cur, max := 0, 0
	for _, e := range evs {
		cur += e.k
		max = maxInt(max, cur)
	}
	return max
}

func contarExcedentes(d *DadosGrupo) int {
	n := 0
	for _, nv := range d.Navios {
		if nv.Excedente {
			n++
		}
	}
	return n
}

// Fila virtual com reserva ilimitada e chamada instantânea é o mesmo que
// não ter fila virtual.
func TestChamadaSemLimiteEhOMesmoQueSemChamada(t *testing.T) {
	for _, d := range append(paranagua(t), grupoPequeno()) {
		sc := Cenario{Nome: "b", Alocacao: AlocPorBerco, Ordem: OrdemReal}
		a := Replay{Dados: d, Cenario: sc}.Rodar()
		b := Replay{Dados: d, Cenario: sc, Chamada: &Chamada{Reserva: 1 << 20, Antecedencia: 0}}.Rodar()
		for i := range d.Navios {
			if a.Atracou[i] != b.Atracou[i] {
				t.Fatalf("%s: navio %s atracou %.2f sem chamada e %.2f com", d.Grupo.Nome, d.Navios[i].ID, a.Atracou[i], b.Atracou[i])
			}
		}
	}
}

func TestReservaCalculada(t *testing.T) {
	c := &Chamada{Antecedencia: 48}
	// grãos: 72 h no berço, 48 h de antecedência → atende 0,67 navio; 1+1
	if got := c.reservaPara(1, 72); got != 2 {
		t.Errorf("grãos: reserva %d, esperava 2", got)
	}
	// contêiner: 16 h no berço → atende 3 navios durante a chamada; 3+1
	if got := c.reservaPara(1, 16); got != 4 {
		t.Errorf("contêiner: reserva %d, esperava 4", got)
	}
	if got := (&Chamada{Reserva: 3, Antecedencia: 48}).reservaPara(2, 16); got != 3 {
		t.Errorf("reserva fixa deveria ser respeitada, veio %d", got)
	}
}

// A previsão na chegada de um navio não pode mudar se o futuro mudar.
func TestPrevisaoNaoOlhaOFuturo(t *testing.T) {
	d := grupoPequeno()
	t0 := 25.0 // B acabou de chegar; A está no berço; C fundeado
	antes := EstadoEm(d, t0, -1)

	// bagunça tudo o que acontece depois de t0
	for i := range d.Navios {
		n := &d.Navios[i]
		if n.Chegada > t0 {
			n.Chegada += 1000
		}
		if n.Atracacao > t0 {
			n.Atracacao += 500
		}
		if n.Desatracacao > t0 {
			n.Desatracacao += 700
		}
	}
	// a mediana de serviço usa só quem já saiu; quem está no berço conta
	// só o quanto já ficou
	depois := EstadoEm(d, t0, -1)
	if antes != depois {
		t.Fatalf("o estado em t=%v mudou quando o futuro mudou:\nantes  %+v\ndepois %+v", t0, antes, depois)
	}
	if antes.Fila != 2 { // C e B fundeados
		t.Errorf("fila = %d, esperava 2", antes.Fila)
	}
}

func TestMinimosQuadradosRecuperaOsPesos(t *testing.T) {
	var X [][]float64
	var y []float64
	for i := range 50 {
		a, b := float64(i), float64((i*7)%11)
		X = append(X, []float64{1, a, b})
		y = append(y, 3+2*a-0.5*b)
	}
	beta := minimosQuadrados(X, y)
	for k, want := range []float64{3, 2, -0.5} {
		if math.Abs(beta[k]-want) > 1e-9 {
			t.Fatalf("β = %v, esperava [3 2 −0.5]", beta)
		}
	}
	if minimosQuadrados(X[:10], y[:10]) != nil {
		t.Error("com menos de 30 observações deveria recusar")
	}
}

func TestTempoCheio(t *testing.T) {
	c := curva{t: []float64{0, 10, 20, 30}, v: []int{1, 2, 1, 0}}
	casos := []struct {
		a, b float64
		cap  int
		want float64
	}{
		{0, 40, 2, 10},  // cheio de 10 a 20
		{15, 25, 2, 5},  // metade dentro
		{0, 40, 1, 30},  // pelo menos um de 0 a 30
		{-5, 5, 1, 5},   // antes do primeiro evento: vazio
		{10, 20, 2, 10}, // exatamente o degrau
	}
	for _, c2 := range casos {
		if got := c.tempoCheio(c2.a, c2.b, c2.cap); math.Abs(got-c2.want) > 1e-9 {
			t.Errorf("tempoCheio(%v, %v, %d) = %v, esperava %v", c2.a, c2.b, c2.cap, got, c2.want)
		}
	}
}

func TestLeitura(t *testing.T) {
	d := Diagnostico{Real: Resumo{Navios: 10, EsperaMediana: 200, Ocupacao: 0.9}, BercoCheio: 0.85}
	if !strings.HasPrefix(d.Leitura(), "congestionamento") {
		t.Errorf("berços cheios e ocupados: %q", d.Leitura())
	}
	d.BercoCheio = 0.1
	if !strings.HasPrefix(d.Leitura(), "não é falta de berço") {
		t.Errorf("berço livre durante a espera: %q", d.Leitura())
	}
	d.Real.EsperaMediana = 10
	if !strings.HasPrefix(d.Leitura(), "espera baixa") {
		t.Errorf("espera curta: %q", d.Leitura())
	}
}

func TestConfig(t *testing.T) {
	base := `porto: X
anos: [2024]
periodo: {inicio: 2024-01-01, fim: 2025-01-01}
grupos:
  - nome: g
    bercos: [B1]
`
	casos := []struct {
		extra, erro string
	}{
		{"", ""},
		{"cenarios:\n  - nome: c\n    bercos_extra: 1\n", "exige alocacao compartilhada"},
		{"cenarios:\n  - nome: c\n    alocacao: talvez\n", "alocacao"},
		{"cenarios:\n  - nome: c\n    grupo: h\n", "não existe"},
		{"cenarios:\n  - nome: c\n    operacao: 0\n", "tempo zero"},
		{"treino: {inicio: 2023-06-01, fim: 2024-06-01}\n", "antes do periodo"},
		{"custo: 3\n", "field custo not found"},
		{"  - nome: h\n    bercos: [B1]\n", "está em"},
	}
	for _, c := range casos {
		dir := t.TempDir()
		p := filepath.Join(dir, "p.yaml")
		if err := os.WriteFile(p, []byte(base+c.extra), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Carregar(p)
		switch {
		case c.erro == "" && err != nil:
			t.Errorf("%q: erro inesperado %v", c.extra, err)
		case c.erro != "" && (err == nil || !strings.Contains(err.Error(), c.erro)):
			t.Errorf("%q: esperava erro com %q, veio %v", c.extra, c.erro, err)
		case c.erro == "" && cfg.PastaDados() != filepath.Join(dir, "dados/antaq"):
			t.Errorf("pasta dos dados relativa ao YAML: %s", cfg.PastaDados())
		}
	}

	// ocioso: 0 é um cenário válido (eliminar o tempo ocioso), não "sem
	// mudança"
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	os.WriteFile(p, []byte(base+"cenarios:\n  - nome: c\n    ocioso: 0\n"), 0o644)
	cfg, err := Carregar(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, oc, _ := cfg.Cenarios[0].Fatores(); oc != 0 {
		t.Errorf("ocioso: 0 virou %v", oc)
	}
}

func TestCapacidadeNosExtremos(t *testing.T) {
	d := grupoPequeno()
	c := CalcularCapacidade(d, Base(), 1e9)
	if want := c.HojeMes * Niveis[len(Niveis)-1]; math.Abs(c.Otimista-want) > 1e-9 {
		t.Errorf("com meta infinita deveria caber o maior nível: %v, esperava %v", c.Otimista, want)
	}
	if c := CalcularCapacidade(d, Base(), 1e-9); c.Otimista != 0 || c.Conservador != 0 {
		t.Errorf("com meta zero não cabe nada: %+v", c)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
