package scenario

import (
	"math"
	"testing"

	"github.com/eduardomilani8/simterminal/internal/stats"
	"github.com/eduardomilani8/simterminal/internal/terminal"
)

func mapConfig() terminal.Config {
	return terminal.Config{
		Horizon:       720,
		Warmup:        60,
		HourlyProfile: []float64{4, 12, 28, 31, 22, 15, 9, 6, 6, 8, 5, 3},
		EntryScales:   1,
		ExitScales:    1,
		Docks:         12,
		Weighing:      terminal.Triangular{Min: 3, Mode: 4, Max: 7},
		Cargo: []terminal.Cargo{
			{Name: "granel", Unload: terminal.Triangular{Min: 35, Mode: 48, Max: 95}, Share: 0.5},
			{Name: "paletizada", Unload: terminal.Triangular{Min: 18, Mode: 25, Max: 40}, Share: 0.5},
		},
	}
}

// TestHalfWidthShrinksWithSqrtR é a âncora do dia 4: a meia-largura do
// IC cai proporcional a 1/√R. Quadruplicou as réplicas, o erro caiu pela
// metade.
func TestHalfWidthShrinksWithSqrtR(t *testing.T) {
	cfg := mapConfig()
	small := Run("R", cfg, Seeds(1, 25))
	large := Run("4R", cfg, Seeds(1000, 100))

	ratio := large.MeanTime.HalfWidth / small.MeanTime.HalfWidth
	// esperado 0,5; a própria estimativa do desvio padrão com 25 réplicas
	// oscila, então a tolerância é larga — mas não aceita "não mudou"
	if ratio < 0.35 || ratio > 0.7 {
		t.Errorf("meia-largura foi de %.1f para %.1f min (razão %.2f), esperava ~0,5",
			small.MeanTime.HalfWidth, large.MeanTime.HalfWidth, ratio)
	}
	if math.Abs(small.MeanTime.Mean-large.MeanTime.Mean) > small.MeanTime.HalfWidth+large.MeanTime.HalfWidth {
		t.Errorf("os dois intervalos nem se tocam: %.1f ± %.1f e %.1f ± %.1f",
			small.MeanTime.Mean, small.MeanTime.HalfWidth, large.MeanTime.Mean, large.MeanTime.HalfWidth)
	}
}

// TestReplicasAreDeterministic: rodar em paralelo não pode mudar nada.
func TestReplicasAreDeterministic(t *testing.T) {
	seeds := Seeds(7, 16)
	a := Run("a", mapConfig(), seeds)
	b := Run("b", mapConfig(), seeds)
	for i := range seeds {
		if a.Results[i].TimeInSystem.Mean() != b.Results[i].TimeInSystem.Mean() {
			t.Fatalf("réplica %d mudou entre execuções", i)
		}
	}
	for i, r := range a.Results {
		if r.Seed != seeds[i] {
			t.Fatalf("réplica %d com semente %d, esperava %d", i, r.Seed, seeds[i])
		}
	}
}

// TestCommonRandomNumbersShrinkDiffCI: com as mesmas sementes nos dois
// cenários, o IC da diferença é bem mais estreito do que tratando as
// rodadas como independentes.
func TestCommonRandomNumbersShrinkDiffCI(t *testing.T) {
	seeds := Seeds(1, 30)
	base := Run("atual", mapConfig(), seeds)
	moreCfg := mapConfig()
	moreCfg.EntryScales = 2
	more := Run("+1 balança", moreCfg, seeds)

	paired := Diff(&base, &more, MetricMeanTime)
	unpaired := stats.UnpairedDiffCI(base.Per(MetricMeanTime), more.Per(MetricMeanTime))

	if paired.Lo() <= 0 {
		t.Errorf("a balança extra deveria reduzir o tempo com confiança: Δ = %.1f ± %.1f",
			paired.Mean, paired.HalfWidth)
	}
	if paired.HalfWidth >= unpaired.HalfWidth {
		t.Errorf("IC pareado ±%.1f não ficou mais estreito que o não pareado ±%.1f",
			paired.HalfWidth, unpaired.HalfWidth)
	}
}

func TestBottleneckAndSaturation(t *testing.T) {
	seeds := Seeds(1, 10)

	s := Run("atual", mapConfig(), seeds)
	if s.Bottleneck != terminal.ResEntryScale {
		t.Errorf("gargalo = %q, esperava a balança de entrada", s.Bottleneck)
	}
	if len(s.Saturated) != 0 {
		t.Errorf("carga < 1 não deveria ser marcada como saturada: %v", s.Saturated)
	}

	cfg := mapConfig()
	cfg.ExitScales = 0 // uma balança só, fazendo entrada e saída
	s = Run("balança única", cfg, seeds)
	if len(s.Saturated) != 1 || s.Saturated[0] != terminal.ResSharedScale {
		t.Errorf("saturados = %v, esperava só a balança única", s.Saturated)
	}

	cfg = mapConfig()
	cfg.EntryScales, cfg.ExitScales, cfg.Docks = 20, 20, 100
	s = Run("folgado", cfg, seeds)
	if s.Bottleneck != "" {
		t.Errorf("com folga em tudo não deveria haver gargalo, veio %q", s.Bottleneck)
	}
}
