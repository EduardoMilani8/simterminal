package terminal

import (
	"math"
	"testing"
)

// baseConfig é o terminal do mapa do projeto: pico de manhã, uma balança
// de cada lado, granel e paletizada meio a meio.
func baseConfig() Config {
	return Config{
		Horizon:       720,
		Warmup:        0,
		HourlyProfile: []float64{4, 12, 28, 31, 22, 15, 9, 6, 6, 8, 5, 3},
		EntryScales:   1,
		ExitScales:    1,
		Docks:         12,
		Weighing:      Triangular{Min: 3, Mode: 4, Max: 7},
		Cargo: []Cargo{
			{Name: "granel", Unload: Triangular{Min: 35, Mode: 48, Max: 95}, Share: 0.5},
			{Name: "paletizada", Unload: Triangular{Min: 18, Mode: 25, Max: 40}, Share: 0.5},
		},
	}
}

// TestLittlesLaw é a âncora do dia 3: L = λ × W. Sem warm-up e rodando
// até o último caminhão sair, as duas contas medem os mesmos
// caminhão-minutos por caminhos diferentes — têm que bater exatamente.
// Se não bater, algum caminhão entrou ou saiu da contabilidade errado.
func TestLittlesLaw(t *testing.T) {
	cfgs := map[string]Config{"base": baseConfig()}
	apertado := baseConfig()
	apertado.Docks = 4
	apertado.ExitScales = 0
	cfgs["saturado, balança única"] = apertado

	for name, cfg := range cfgs {
		for seed := int64(1); seed <= 5; seed++ {
			r := Run(cfg, seed)
			L := r.MeanInSystem
			lambda := r.ArrivalRate(cfg.Warmup)
			W := r.TimeInSystem.Mean()
			if math.Abs(L-lambda*W)/L > 1e-9 {
				t.Errorf("%s, semente %d: L = %.4f, λW = %.4f", name, seed, L, lambda*W)
			}
		}
	}
}

// TestNoTruckLost: todo caminhão que entra, sai. Sumiu caminhão, tem bug.
func TestNoTruckLost(t *testing.T) {
	cfg := baseConfig()
	cfg.Warmup = 60
	for seed := int64(1); seed <= 10; seed++ {
		r := Run(cfg, seed)
		if r.Arrived == 0 || r.Arrived != r.Completed {
			t.Errorf("semente %d: chegaram %d, saíram %d", seed, r.Arrived, r.Completed)
		}
		if r.TimeInSystem.N() != r.Completed || len(r.Trucks) != r.Completed {
			t.Errorf("semente %d: contagens inconsistentes", seed)
		}
	}
}

// TestLightLoad: longe da saturação ninguém espera, então o tempo no
// terminal é só a soma dos tempos de serviço, e as chegadas batem com o
// perfil.
func TestLightLoad(t *testing.T) {
	cfg := baseConfig()
	cfg.EntryScales, cfg.ExitScales, cfg.Docks = 50, 50, 200

	const seeds = 200
	arrivals, timeSum, n := 0.0, 0.0, 0
	for seed := int64(1); seed <= seeds; seed++ {
		r := Run(cfg, seed)
		arrivals += float64(r.Arrived)
		for _, c := range r.Trucks {
			timeSum += c.TimeInSystem()
			n++
		}
		for _, rr := range r.Resources {
			if rr.MeanWait > 1e-9 {
				t.Fatalf("semente %d: espera em %s com capacidade sobrando", seed, rr.Name)
			}
		}
	}

	want := cfg.ExpectedArrivals()
	got := arrivals / seeds
	if tol := 4 * math.Sqrt(want/seeds); math.Abs(got-want) > tol {
		t.Errorf("chegadas por dia = %.1f, esperava %.1f (±%.1f)", got, want, tol)
	}

	wantW := 2*cfg.Weighing.Mean() + cfg.MeanUnload()
	if gotW := timeSum / float64(n); math.Abs(gotW-wantW)/wantW > 0.02 {
		t.Errorf("tempo médio no terminal = %.1f, esperava %.1f", gotW, wantW)
	}
}

func TestMarksAreInOrder(t *testing.T) {
	r := Run(baseConfig(), 3)
	order := []string{MarkArrival, MarkWeighInStart, MarkWeighInEnd, MarkUnloadStart,
		MarkUnloadEnd, MarkWeighOutStart, MarkExit}
	for _, c := range r.Trucks {
		for i := 1; i < len(order); i++ {
			a, okA := c.Marcos[order[i-1]]
			b, okB := c.Marcos[order[i]]
			if !okA || !okB || b < a {
				t.Fatalf("caminhão %d: marcos fora de ordem: %v", c.ID, c.Marcos)
			}
		}
	}
}

func TestSameSeedSameResult(t *testing.T) {
	a, b := Run(baseConfig(), 11), Run(baseConfig(), 11)
	if a.TimeInSystem.Mean() != b.TimeInSystem.Mean() || a.EndTime != b.EndTime {
		t.Error("mesma semente deveria dar o mesmo resultado")
	}
}

// TestCommonRandomNumbers: mudar a estrutura do terminal não pode mudar
// quem chega, quando chega, nem quanto cada caminhão demora na doca.
func TestCommonRandomNumbers(t *testing.T) {
	base := baseConfig()
	more := baseConfig()
	more.EntryScales = 2

	a, b := Run(base, 5), Run(more, 5)
	byID := func(r Result) map[int]*Caminhao {
		m := map[int]*Caminhao{}
		for _, c := range r.Trucks {
			m[c.ID] = c
		}
		return m
	}
	ta, tb := byID(a), byID(b)
	if len(ta) != len(tb) {
		t.Fatalf("%d caminhões num cenário, %d no outro", len(ta), len(tb))
	}
	for id, ca := range ta {
		cb := tb[id]
		if ca.ChegadaEm != cb.ChegadaEm || ca.TipoCarga != cb.TipoCarga || ca.descarga != cb.descarga {
			t.Fatalf("caminhão %d difere entre cenários", id)
		}
	}
	if a.TimeInSystem.Mean() <= b.TimeInSystem.Mean() {
		t.Errorf("segunda balança deveria reduzir o tempo: %.0f → %.0f min",
			a.TimeInSystem.Mean(), b.TimeInSystem.Mean())
	}
}

func TestSharedScaleServesBothWays(t *testing.T) {
	cfg := baseConfig()
	cfg.ExitScales = 0
	r := Run(cfg, 2)
	if len(r.Resources) != 2 {
		t.Fatalf("esperava balança única + docas, veio %d recursos", len(r.Resources))
	}
	if _, ok := r.Resource(ResSharedScale); !ok {
		t.Fatal("recurso de balança compartilhada não encontrado")
	}
}

func TestScheduledArrivals(t *testing.T) {
	cfg := baseConfig()
	cfg.Scheduled = 1
	cfg.ScheduleTolerance = 10
	r := Run(cfg, 4)

	want := int(math.Round(cfg.ExpectedArrivals()))
	if r.Arrived != want {
		t.Errorf("com 100%% agendado chegaram %d, esperava %d", r.Arrived, want)
	}
	for _, c := range r.Trucks {
		if !c.Agendado || c.ChegadaEm < 0 || c.ChegadaEm >= cfg.Horizon {
			t.Fatalf("caminhão %d: agendado=%v chegada=%.1f", c.ID, c.Agendado, c.ChegadaEm)
		}
	}

	// agendar metade achata o pico da manhã: a fila tem que cair
	half := baseConfig()
	half.Scheduled = 0.5
	half.ScheduleTolerance = 10
	var none, some float64
	for seed := int64(1); seed <= 20; seed++ {
		a, b := Run(baseConfig(), seed), Run(half, seed)
		none += a.TimeInSystem.Mean()
		some += b.TimeInSystem.Mean()
	}
	if some >= none {
		t.Errorf("agendar metade deveria reduzir o tempo médio: %.0f → %.0f min", none/20, some/20)
	}
}

func TestWarmupExcludesEarlyTrucks(t *testing.T) {
	cfg := baseConfig()
	cfg.Warmup = 120
	r := Run(cfg, 1)
	for _, c := range r.Trucks {
		if c.ChegadaEm < cfg.Warmup {
			t.Fatalf("caminhão %d chegou em %.1f, antes do fim do warm-up", c.ID, c.ChegadaEm)
		}
	}
}

func TestOfferedLoad(t *testing.T) {
	cfg := baseConfig()
	load := cfg.OfferedLoad()
	// 149 caminhões × 4,67 min ÷ 720 min ≈ 0,97
	if got := load[ResEntryScale]; math.Abs(got-0.966) > 0.01 {
		t.Errorf("carga na balança de entrada = %.3f, esperava ~0.966", got)
	}
	cfg.ExitScales = 0
	if got := cfg.OfferedLoad()[ResSharedScale]; got < 1.9 {
		t.Errorf("balança única fazendo entrada e saída deveria ter carga ~1.93, veio %.3f", got)
	}
}

func TestValidate(t *testing.T) {
	if err := func() error { c := baseConfig(); return c.Validate() }(); err != nil {
		t.Fatalf("config base inválida: %v", err)
	}
	broken := []func(*Config){
		func(c *Config) { c.Horizon = 0 },
		func(c *Config) { c.Warmup = c.Horizon },
		func(c *Config) { c.HourlyProfile = nil },
		func(c *Config) { c.HourlyProfile = []float64{0, 0} },
		func(c *Config) { c.EntryScales = 0 },
		func(c *Config) { c.Docks = 0 },
		func(c *Config) { c.Weighing = Triangular{Min: 5, Mode: 4, Max: 7} },
		func(c *Config) { c.Cargo[0].Share = 0.7 },
		func(c *Config) { c.Scheduled = 1.5 },
	}
	for i, breakIt := range broken {
		c := baseConfig()
		breakIt(&c)
		if c.Validate() == nil {
			t.Errorf("caso %d: configuração quebrada passou na validação", i)
		}
	}
}
