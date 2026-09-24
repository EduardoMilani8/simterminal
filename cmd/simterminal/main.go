package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eduardomilani8/simterminal/internal/des"
)

// Demo do dia 1: uma balança só, caminhões chegando ao longo de um turno.
// Ainda não é o terminal completo — é o motor de eventos rodando de
// ponta a ponta para provar que a base funciona.
func main() {
	var (
		seed     = flag.Int64("seed", 1, "semente do gerador aleatório")
		horizon  = flag.Float64("horizonte", 10*60, "duração do turno em minutos")
		interval = flag.Float64("intervalo", 6, "intervalo médio entre chegadas em minutos")
		service  = flag.Float64("pesagem", 4, "tempo médio de pesagem em minutos")
		trace    = flag.Bool("trace", false, "imprime cada evento executado")
	)
	flag.Parse()

	e := des.New(*seed)
	if *trace {
		e.SetTrace(os.Stdout)
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

	fmt.Printf("turno de %.0f min | semente %d | %d eventos processados\n\n",
		*horizon, *seed, e.EventsProcessed())
	espera := balanca.Waits()
	fmt.Printf("caminhões pesados      %d\n", espera.N())
	fmt.Printf("espera média na fila   %.1f min\n", espera.Mean())
	fmt.Printf("espera p95             %.1f min\n", espera.Percentile(95))
	fmt.Printf("pior espera            %.1f min\n", espera.Max())
	fmt.Printf("fila máxima            %d caminhões\n", balanca.MaxQueue())
	fmt.Printf("utilização da balança  %.0f%%\n", balanca.Utilization()*100)
}
