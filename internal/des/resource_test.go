package des

import (
	"math"
	"testing"
)

// mm1Resource é a mesma M/M/1 de mm1Manual, mas usando Resource. Os
// sorteios acontecem na mesma ordem, então com a mesma semente o
// resultado tem que ser idêntico — não parecido, idêntico.
func mm1Resource(seed int64, meanInterarrival, meanService float64, nCustomers int) (wq, util float64) {
	e := New(seed)
	r := NewResource(e, "servidor", 1)

	nArrived := 0
	var arrive func()
	arrive = func() {
		nArrived++
		r.Request(func(release func()) {
			e.ScheduleIn(e.Exponential(meanService), "fim do atendimento", release)
		})
		if nArrived < nCustomers {
			e.ScheduleIn(e.Exponential(meanInterarrival), "chegada", arrive)
		}
	}

	e.ScheduleAt(0, "chegada", arrive)
	e.Run(0)
	return r.Waits().Mean(), r.Utilization()
}

// TestResourceMM1MatchesManual é a âncora do dia 2: reescrever a M/M/1
// com Resource não pode mudar o resultado. Deu diferente, o recurso tem
// bug.
func TestResourceMM1MatchesManual(t *testing.T) {
	const n = 300_000
	manual := mm1Manual(42, 10, 7, n)
	viaResource, util := mm1Resource(42, 10, 7, n)

	if math.Abs(manual-viaResource) > 1e-9 {
		t.Errorf("espera com Resource = %.6f, manual = %.6f", viaResource, manual)
	}
	// rho = 7/10: o servidor tem que ficar ocupado ~70% do tempo.
	if math.Abs(util-0.7) > 0.01 {
		t.Errorf("utilização = %.3f, esperava ~0.700", util)
	}
}

// TestResourceMMcMatchesErlangC valida a capacidade N contra a fórmula
// de Erlang C para uma fila M/M/2.
func TestResourceMMcMatchesErlangC(t *testing.T) {
	const (
		c                = 2
		meanInterarrival = 5.0
		meanService      = 7.0
		n                = 300_000
	)

	e := New(7)
	r := NewResource(e, "docas", c)
	nArrived := 0
	var arrive func()
	arrive = func() {
		nArrived++
		r.Request(func(release func()) {
			e.ScheduleIn(e.Exponential(meanService), "fim", release)
		})
		if nArrived < n {
			e.ScheduleIn(e.Exponential(meanInterarrival), "chegada", arrive)
		}
	}
	e.ScheduleAt(0, "chegada", arrive)
	e.Run(0)

	lambda, mu := 1/meanInterarrival, 1/meanService
	a := lambda / mu // carga oferecida em erlangs
	rho := a / c

	// Erlang C: probabilidade de quem chega ter que esperar.
	sum, term := 0.0, 1.0
	for k := 0; k < c; k++ {
		if k > 0 {
			term *= a / float64(k)
		}
		sum += term
	}
	top := term * a / float64(c) / (1 - rho) // a^c / c! / (1-rho)
	pWait := top / (sum + top)
	wantWq := pWait / (float64(c)*mu - lambda)

	if got := r.Waits().Mean(); math.Abs(got-wantWq)/wantWq > 0.05 {
		t.Errorf("espera M/M/%d = %.2f min, Erlang C = %.2f min", c, got, wantWq)
	}
	if got := r.Utilization(); math.Abs(got-rho) > 0.01 {
		t.Errorf("utilização = %.3f, esperava %.3f", got, rho)
	}
}

func TestResourceFIFOAndCapacity(t *testing.T) {
	e := New(1)
	r := NewResource(e, "doca", 2)
	var order []int

	for i := 0; i < 4; i++ {
		i := i
		r.Request(func(release func()) {
			order = append(order, i)
			e.ScheduleIn(10, "fim", release)
		})
	}
	if r.InUse() != 2 || r.QueueLen() != 2 {
		t.Fatalf("em uso %d, fila %d; esperava 2 e 2", r.InUse(), r.QueueLen())
	}
	e.Run(0)

	want := []int{0, 1, 2, 3}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("ordem de atendimento %v, esperava %v", order, want)
		}
	}
	if r.MaxQueue() != 2 {
		t.Errorf("fila máxima = %d, esperava 2", r.MaxQueue())
	}
	// dois esperaram 10 min, dois não esperaram
	if got := r.Waits().Mean(); got != 5 {
		t.Errorf("espera média = %v, esperava 5", got)
	}
}

// TestResourceUtilizationIsTimeWeighted: ocupado de 0 a 10 num horizonte
// de 40 é 25%, não importa quantos atendimentos houve.
func TestResourceUtilizationIsTimeWeighted(t *testing.T) {
	e := New(1)
	r := NewResource(e, "balança", 1)
	r.Request(func(release func()) { e.ScheduleIn(10, "fim", release) })
	e.Run(40)

	if got := r.Utilization(); math.Abs(got-0.25) > 1e-12 {
		t.Errorf("utilização = %v, esperava 0.25", got)
	}
}

func TestResourceResetStatsDropsWarmup(t *testing.T) {
	e := New(1)
	r := NewResource(e, "balança", 1)
	r.Request(func(release func()) { e.ScheduleIn(10, "fim", release) })
	r.Request(func(release func()) { e.ScheduleIn(10, "fim", release) })
	e.Run(20)
	r.ResetStats()
	e.Run(40)

	if got := r.Utilization(); got != 0 {
		t.Errorf("utilização após reset = %v, esperava 0", got)
	}
	if r.Waits().N() != 0 {
		t.Errorf("esperas após reset = %d, esperava 0", r.Waits().N())
	}
}

func TestResourceDoubleReleasePanics(t *testing.T) {
	e := New(1)
	r := NewResource(e, "balança", 1)
	defer func() {
		if recover() == nil {
			t.Error("liberar duas vezes deveria dar panic")
		}
	}()
	r.Request(func(release func()) {
		release()
		release()
	})
}
