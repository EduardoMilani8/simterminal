package report

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/eduardomilani8/simterminal/internal/scenario"
	"github.com/eduardomilani8/simterminal/internal/stats"
)

func TestDuration(t *testing.T) {
	cases := map[float64]string{
		0: "0min", 0.4: "0min", 38.2: "38min", 59.6: "1h00", 167: "2h47", 600: "10h00",
		-109: "−1h49", -0.3: "0min",
	}
	for in, want := range cases {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%v) = %q, esperava %q", in, got, want)
		}
	}
}

func TestIntervalFormats(t *testing.T) {
	iv := stats.Interval{Mean: 167, HalfWidth: 11, N: 30}
	if got := DurationCI(iv); got != "2h47 ± 11min" {
		t.Errorf("DurationCI = %q", got)
	}
	if got := SignedDurationCI(stats.Interval{Mean: 3, HalfWidth: 2, N: 30}); got != "+3min ± 2min" {
		t.Errorf("SignedDurationCI = %q", got)
	}
	if got := PercentCI(stats.Interval{Mean: 0.964, HalfWidth: 0.004, N: 30}); got != "96%" {
		t.Errorf("PercentCI com meia-largura desprezível = %q", got)
	}
	if got := CountCI(stats.Interval{Mean: 46.2, HalfWidth: 3.1, N: 30}); got != "46 ± 3" {
		t.Errorf("CountCI = %q", got)
	}
}

func load(t *testing.T, names ...string) []*scenario.Summary {
	t.Helper()
	var ss []*scenario.Summary
	for _, n := range names {
		sc, err := scenario.Load("../../exemplos/" + n + ".yaml")
		if err != nil {
			t.Fatal(err)
		}
		s := scenario.Run(sc.Name, sc.Config, scenario.Seeds(1, 10))
		ss = append(ss, &s)
	}
	return ss
}

func TestWriteRun(t *testing.T) {
	ss := load(t, "atual")
	var b bytes.Buffer
	WriteRun(&b, ss[0])
	out := b.String()
	for _, want := range []string{"atual", "tempo no terminal", "balança de entrada", "docas", "gargalo: balança de entrada"} {
		if !strings.Contains(out, want) {
			t.Errorf("relatório sem %q:\n%s", want, out)
		}
	}
}

func TestWriteCompare(t *testing.T) {
	ss := load(t, "atual", "maisbalanca", "maisdoca", "balancaunica")
	var b bytes.Buffer
	WriteCompare(&b, ss)
	out := b.String()
	for _, want := range []string{"+ 1 balança", "+ 1 doca", "saturado", "mais rápido", `Conclusão: "+ 1 balança" é o melhor`} {
		if !strings.Contains(out, want) {
			t.Errorf("comparação sem %q:\n%s", want, out)
		}
	}
}

func TestWriteCSV(t *testing.T) {
	ss := load(t, "atual", "maisbalanca")
	var b bytes.Buffer
	if err := WriteCSV(&b, ss); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&b).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// cabeçalho + 2 cenários × (5 globais + 3 recursos × 5 métricas)
	if want := 1 + 2*(5+3*5); len(rows) != want {
		t.Errorf("%d linhas, esperava %d", len(rows), want)
	}

	b.Reset()
	if err := WriteTrucksCSV(&b, &ss[0].Results[0]); err != nil {
		t.Fatal(err)
	}
	rows, _ = csv.NewReader(&b).ReadAll()
	if len(rows)-1 != ss[0].Results[0].Completed {
		t.Errorf("%d caminhões no CSV, esperava %d", len(rows)-1, ss[0].Results[0].Completed)
	}
}
