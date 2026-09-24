package des

import (
	"container/heap"
	"fmt"
	"io"
	"math/rand"
)

// Engine é o motor de simulação por eventos discretos.
//
// O relógio não avança de minuto em minuto: ele salta direto para o
// instante do próximo evento agendado. Intervalos em que nada acontece
// custam zero.
type Engine struct {
	now    float64
	queue  eventQueue
	seq    int
	rng    *rand.Rand
	trace  io.Writer
	ran    int
	halted bool
}

// New cria um motor com a semente informada. Mesma semente, mesma
// sequência de números aleatórios, mesmo resultado — indispensável para
// comparar cenários e para depurar.
func New(seed int64) *Engine {
	e := &Engine{rng: rand.New(rand.NewSource(seed))}
	heap.Init(&e.queue)
	return e
}

// Now devolve o instante atual do tempo simulado, em minutos.
func (e *Engine) Now() float64 { return e.now }

// Rand expõe o gerador do motor. Todo sorteio da simulação deve sair
// daqui, nunca do rand global, ou a reprodutibilidade se perde.
func (e *Engine) Rand() *rand.Rand { return e.rng }

// EventsProcessed devolve quantos eventos já foram executados.
func (e *Engine) EventsProcessed() int { return e.ran }

// SetTrace liga o log de eventos. Útil no começo, insuportável depois.
func (e *Engine) SetTrace(w io.Writer) { e.trace = w }

// ScheduleAt agenda um evento para um instante absoluto.
func (e *Engine) ScheduleAt(t float64, name string, action func()) *Event {
	if t < e.now {
		panic(fmt.Sprintf("des: evento %q agendado no passado (t=%.2f, now=%.2f)", name, t, e.now))
	}
	ev := &Event{Time: t, Name: name, Action: action, seq: e.seq}
	e.seq++
	heap.Push(&e.queue, ev)
	return ev
}

// ScheduleIn agenda um evento para daqui a delay minutos.
func (e *Engine) ScheduleIn(delay float64, name string, action func()) *Event {
	return e.ScheduleAt(e.now+delay, name, action)
}

// Halt interrompe o laço após o evento corrente.
func (e *Engine) Halt() { e.halted = true }

// Run executa eventos até esgotar a fila ou até until (em minutos).
// Use until <= 0 para rodar até não sobrar nada agendado.
func (e *Engine) Run(until float64) {
	e.halted = false
	for e.queue.Len() > 0 && !e.halted {
		if until > 0 && e.queue[0].Time > until {
			break
		}
		ev := heap.Pop(&e.queue).(*Event)
		e.now = ev.Time
		e.ran++
		if e.trace != nil {
			fmt.Fprintf(e.trace, "%8.2f  %s\n", e.now, ev.Name)
		}
		if ev.Action != nil {
			ev.Action()
		}
	}
	if until > 0 && e.now < until {
		e.now = until
	}
}
