package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/eduardomilani8/simterminal/internal/des"
)

// cmdDemo é a demo do dia 1: uma balança só, caminhões chegando ao longo
// de um turno. Não é o terminal — é o motor de eventos rodando de ponta a
// ponta, bom para ver o relógio saltar com -trace.
func cmdDemo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		seed     = fs.Int64("seed", 1, "semente do gerador aleatório")
		horizon  = fs.Float64("horizonte", 10*60, "duração do turno em minutos")
		interval = fs.Float64("intervalo", 6, "intervalo médio entre chegadas em minutos")
		service  = fs.Float64("pesagem", 4, "tempo médio de pesagem em minutos")
		trace    = fs.Bool("trace", false, "imprime cada evento executado")
	)
	if fs.Parse(args) != nil {
		return 2
	}

	e := des.New(*seed)
	if *trace {
		e.SetTrace(stdout)
	}

	balanca := des.NewResource(e, "balança", 1)

	var chega func()
	chega = func() {
		balanca.Request(func(release func()) {
			e.ScheduleIn(e.Triangular(*service*0.6, *service, *service*2.2), "fim da pesagem", release)
		})
		if e.Now() < *horizon {
			e.ScheduleIn(e.Exponential(*interval), "chegada", chega)
		}
	}

	e.ScheduleAt(0, "chegada", chega)
	e.Run(0)

	fmt.Fprintf(stdout, "turno de %.0f min | semente %d | %d eventos processados\n\n",
		*horizon, *seed, e.EventsProcessed())
	espera := balanca.Waits()
	fmt.Fprintf(stdout, "caminhões pesados      %d\n", espera.N())
	fmt.Fprintf(stdout, "espera média na fila   %.1f min\n", espera.Mean())
	fmt.Fprintf(stdout, "espera p95             %.1f min\n", espera.Percentile(95))
	fmt.Fprintf(stdout, "pior espera            %.1f min\n", espera.Max())
	fmt.Fprintf(stdout, "fila máxima            %d caminhões\n", balanca.MaxQueue())
	fmt.Fprintf(stdout, "utilização da balança  %.0f%%\n", balanca.Utilization()*100)
	return 0
}
