package des

import "container/heap"

// Event é algo que acontece em um instante específico do tempo simulado.
// Action normalmente agenda novos eventos mais adiante no tempo.
type Event struct {
	Time   float64 // instante em que o evento ocorre (minutos desde o início)
	Name   string  // rótulo para debug/trace
	Action func()  // o que acontece quando o evento é executado

	seq   int // desempate: quem foi agendado primeiro ocorre primeiro
	index int
}

// eventQueue é uma min-heap ordenada por (Time, seq).
type eventQueue []*Event

func (q eventQueue) Len() int { return len(q) }

func (q eventQueue) Less(i, j int) bool {
	if q[i].Time == q[j].Time {
		return q[i].seq < q[j].seq
	}
	return q[i].Time < q[j].Time
}

func (q eventQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *eventQueue) Push(x any) {
	e := x.(*Event)
	e.index = len(*q)
	*q = append(*q, e)
}

func (q *eventQueue) Pop() any {
	old := *q
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	*q = old[:n-1]
	return e
}

var _ heap.Interface = (*eventQueue)(nil)
