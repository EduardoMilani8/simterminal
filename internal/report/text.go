package report

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/eduardomilani8/simterminal/internal/scenario"
	"github.com/eduardomilani8/simterminal/internal/stats"
)

// WriteRun escreve o relatório de um cenário.
func WriteRun(w io.Writer, s *scenario.Summary) {
	fmt.Fprintf(w, "%s — %s, warm-up %s, IC 95%%\n\n", s.Name, seedsLabel(s.Seeds), Duration(s.Config.Warmup))

	writeSaturation(w, s)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  tempo no terminal\t%s\n", DurationCI(s.MeanTime))
	fmt.Fprintf(tw, "  p95 do tempo\t%s\n", DurationCI(s.P95Time))
	fmt.Fprintf(tw, "  fila máx. no pátio\t%s caminhões\n", CountCI(s.MaxQueue))
	fmt.Fprintf(tw, "  caminhões atendidos\t%s\n", CountCI(s.Trucks))
	fmt.Fprintf(tw, "  último caminhão sai\t%s após a abertura (portão fecha em %s)\n",
		DurationCI(s.EndTime), Duration(s.Config.Horizon))
	tw.Flush()
	fmt.Fprintln(w)

	// números alinhados à direita; o nome, completado com espaços até a
	// mesma largura, fica à esquerda
	nameWidth := utf8.RuneCountInString("recurso")
	for _, r := range s.Resources {
		nameWidth = max(nameWidth, utf8.RuneCountInString(r.Name))
	}
	pad := func(name string) string {
		return name + strings.Repeat(" ", nameWidth-utf8.RuneCountInString(name))
	}
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "  %s\tcap.\tcarga\tutilização\tespera média\tfila máx.\t\n", pad("recurso"))
	for _, r := range s.Resources {
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\t%s\t%s\t\n", pad(r.Name), r.Capacity, Percent(r.OfferedLoad),
			PercentCI(r.Utilization), DurationCI(r.MeanWait), CountCI(r.MaxQueue))
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", bottleneckLine(s))
}

// WriteCompare escreve a tabela de decisão entre cenários. O primeiro é
// a referência para as diferenças.
func WriteCompare(w io.Writer, ss []*scenario.Summary) {
	if len(ss) == 0 {
		return
	}
	base := ss[0]
	fmt.Fprintf(w, "%d cenários com as mesmas %s, IC 95%%\n\n", len(ss), seedsLabel(base.Seeds))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Cenário\tRecursos\tTempo médio\tp95\tFila máx\tUtil. balança\tGargalo\n")
	for _, s := range ss {
		mean, p95 := DurationCI(s.MeanTime), Duration(s.P95Time.Mean)
		if len(s.Saturated) > 0 {
			mean, p95 = "saturado", "—"
		}
		util := "—"
		if len(s.Resources) > 0 {
			util = Percent(s.Resources[0].Utilization.Mean)
		}
		gargalo := s.Bottleneck
		if gargalo == "" {
			gargalo = "nenhum"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.0f\t%s\t%s\n",
			s.Name, resourcesLabel(s), mean, p95, s.MaxQueue.Mean, util, gargalo)
	}
	tw.Flush()

	fmt.Fprintln(w)
	for _, s := range ss {
		writeSaturation(w, s)
	}

	if len(ss) < 2 {
		return
	}
	fmt.Fprintf(w, "Tempo médio comparado a %q (pareado, mesmas sementes):\n", base.Name)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range ss[1:] {
		d := scenario.Diff(s, base, scenario.MetricMeanTime)
		verdict := diffVerdict(d)
		if len(s.Saturated) > 0 {
			verdict = "saturado — não compare pela média"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", s.Name, SignedDurationCI(d), verdict)
	}
	tw.Flush()

	fmt.Fprintf(w, "\n%s\n", conclusion(ss))
}

func writeSaturation(w io.Writer, s *scenario.Summary) {
	for _, name := range s.Saturated {
		r, _ := s.Resource(name)
		fmt.Fprintf(w, "  ⚠ %s: saturado — %s recebe %s da própria capacidade. A fila cresce\n"+
			"    sem parar e as médias passam a depender do horizonte escolhido, não do sistema.\n\n",
			s.Name, name, Percent(r.OfferedLoad))
	}
}

func bottleneckLine(s *scenario.Summary) string {
	if s.Bottleneck == "" {
		return "gargalo: nenhum recurso faz os caminhões esperarem de forma relevante."
	}
	r, _ := s.Resource(s.Bottleneck)
	return fmt.Sprintf("gargalo: %s — espera média de %s por caminhão, utilização de %s.",
		r.Name, Duration(r.MeanWait.Mean), Percent(r.Utilization.Mean))
}

func diffVerdict(d stats.Interval) string {
	switch {
	case d.Hi() < 0:
		return "mais rápido"
	case d.Lo() > 0:
		return "mais lento"
	default:
		return "sem diferença detectável"
	}
}

// conclusion aponta o melhor cenário não saturado e diz se a vantagem
// sobre a referência é estatisticamente defensável.
func conclusion(ss []*scenario.Summary) string {
	base := ss[0]
	var best *scenario.Summary
	for _, s := range ss {
		if len(s.Saturated) > 0 {
			continue
		}
		if best == nil || s.MeanTime.Mean < best.MeanTime.Mean {
			best = s
		}
	}
	switch {
	case best == nil:
		return "Conclusão: todos os cenários estão saturados; nenhum resultado é confiável."
	case best == base:
		return fmt.Sprintf("Conclusão: nenhum cenário reduz o tempo médio em relação a %q.", base.Name)
	}
	d := scenario.Diff(best, base, scenario.MetricMeanTime)
	if d.Hi() >= 0 {
		return fmt.Sprintf("Conclusão: %q tem o menor tempo médio, mas a diferença para %q não é\n"+
			"significativa com %d réplicas. Rode mais réplicas antes de decidir.", best.Name, base.Name, d.N)
	}
	return fmt.Sprintf("Conclusão: %q é o melhor — %s a menos por caminhão que %q.",
		best.Name, DurationCI(stats.Interval{Mean: -d.Mean, HalfWidth: d.HalfWidth, N: d.N}), base.Name)
}

func seedsLabel(seeds []int64) string {
	switch len(seeds) {
	case 0:
		return "0 réplicas"
	case 1:
		return fmt.Sprintf("1 réplica (semente %d)", seeds[0])
	}
	return fmt.Sprintf("%d réplicas (sementes %d–%d)", len(seeds), seeds[0], seeds[len(seeds)-1])
}

func resourcesLabel(s *scenario.Summary) string {
	c := s.Config
	var b strings.Builder
	if c.ExitScales == 0 {
		fmt.Fprintf(&b, "%d bal. única", c.EntryScales)
	} else {
		fmt.Fprintf(&b, "%d+%d bal.", c.EntryScales, c.ExitScales)
	}
	fmt.Fprintf(&b, ", %d docas", c.Docks)
	if c.Scheduled > 0 {
		fmt.Fprintf(&b, ", %s agend.", Percent(c.Scheduled))
	}
	return b.String()
}
