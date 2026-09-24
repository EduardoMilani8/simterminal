package des

import (
	"math"
	"testing"
)

// TestEngineOrdersEventsByTime garante o básico: os eventos saem em
// ordem cronológica, independentemente da ordem em que foram agendados.
func TestEngineOrdersEventsByTime(t *testing.T) {
	e := New(1)
	var got []float64
	for _, at := range []float64{30, 5, 20, 5, 10} {
		at := at
		e.ScheduleAt(at, "x", func() { got = append(got, e.Now()) })
	}
	e.Run(0)

	want := []float64{5, 5, 10, 20, 30}
	if len(got) != len(want) {
		t.Fatalf("executou %d eventos, esperava %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("evento %d em t=%.0f, esperava t=%.0f", i, got[i], want[i])
		}
	}
}

// TestMM1MatchesAnalyticFormula é o teste que realmente importa: uma
// fila M/M/1 simulada tem que bater com a fórmula fechada conhecida.
// Se o motor estiver errado, é aqui que aparece.
//
// Chegadas exponenciais com média 10 min, atendimento exponencial com
// média 7 min, um servidor só.
//
//	lambda = 1/10, mu = 1/7, rho = 0.7
//	Wq (espera média na fila) = rho / (mu - lambda)
func TestMM1MatchesAnalyticFormula(t *testing.T) {
	const (
		meanInterarrival = 10.0
		meanService      = 7.0
		nCustomers       = 300_000
	)

	gotWq := mm1Manual(42, meanInterarrival, meanService, nCustomers)

	lambda := 1 / meanInterarrival
	mu := 1 / meanService
	rho := lambda / mu
	wantWq := rho / (mu - lambda)

	if rel := math.Abs(gotWq-wantWq) / wantWq; rel > 0.05 {
		t.Errorf("espera média simulada = %.2f min, fórmula = %.2f min (erro %.1f%%)",
			gotWq, wantWq, rel*100)
	}
}

// mm1Manual simula uma M/M/1 controlando fila e servidor na mão, sem
// Resource, e devolve a espera média na fila.
func mm1Manual(seed int64, meanInterarrival, meanService float64, nCustomers int) float64 {
	e := New(seed)

	var (
		queue      []float64 // instantes de chegada dos que estão esperando
		busy       bool
		totalWait  float64
		nCompleted int
		nArrived   int
		startNext  func()
		arrive     func()
	)

	startNext = func() {
		if len(queue) == 0 {
			busy = false
			return
		}
		arrivedAt := queue[0]
		queue = queue[1:]
		busy = true
		totalWait += e.Now() - arrivedAt
		nCompleted++
		e.ScheduleIn(e.Exponential(meanService), "fim do atendimento", startNext)
	}

	arrive = func() {
		nArrived++
		queue = append(queue, e.Now())
		if !busy {
			startNext()
		}
		if nArrived < nCustomers {
			e.ScheduleIn(e.Exponential(meanInterarrival), "chegada", arrive)
		}
	}

	e.ScheduleAt(0, "chegada", arrive)
	e.Run(0)

	return totalWait / float64(nCompleted)
}
