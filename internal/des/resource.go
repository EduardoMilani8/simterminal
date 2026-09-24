package des

import (
	"fmt"

	"github.com/eduardomilani8/simterminal/internal/stats"
)

// Resource é algo disputado e com capacidade limitada: uma balança, um
// conjunto de docas, um guichê.
//
// Quem chama Request é atendido na hora se houver vaga, ou entra numa
// fila FIFO. O callback só dispara quando chega a vez, e recebe a função
// release, que devolve o recurso e puxa o próximo da fila.
//
//	r := des.NewResource(e, "balança", 1)
//	r.Request(func(release func()) {
//	    e.ScheduleIn(tempoPesagem, "fim pesagem", release)
//	})
type Resource struct {
	e        *Engine
	name     string
	capacity int
	inUse    int
	queue    []pending

	busy   stats.TimeWeighted // servidores ocupados ao longo do tempo
	queued stats.TimeWeighted // tamanho da fila ao longo do tempo
	waits  stats.Series       // espera de cada atendimento iniciado
}

type pending struct {
	since float64
	start func(release func())
}

// NewResource cria um recurso com capacity servidores idênticos.
func NewResource(e *Engine, name string, capacity int) *Resource {
	if capacity < 1 {
		panic(fmt.Sprintf("des: recurso %q com capacidade %d", name, capacity))
	}
	r := &Resource{e: e, name: name, capacity: capacity}
	r.busy.Reset(e.Now())
	r.queued.Reset(e.Now())
	return r
}

// Request pede uma unidade do recurso. start é chamado assim que houver
// vaga — na hora, se houver agora — e deve chamar release exatamente uma
// vez quando terminar de usar.
func (r *Resource) Request(start func(release func())) {
	if r.inUse < r.capacity {
		r.grant(r.e.Now(), start)
		return
	}
	r.queue = append(r.queue, pending{since: r.e.Now(), start: start})
	r.queued.Set(r.e.Now(), float64(len(r.queue)))
}

func (r *Resource) grant(since float64, start func(release func())) {
	now := r.e.Now()
	r.inUse++
	r.busy.Set(now, float64(r.inUse))
	r.waits.Add(now - since)

	released := false
	start(func() {
		if released {
			panic(fmt.Sprintf("des: recurso %q liberado duas vezes", r.name))
		}
		released = true
		r.release()
	})
}

func (r *Resource) release() {
	now := r.e.Now()
	r.inUse--
	r.busy.Set(now, float64(r.inUse))
	if len(r.queue) == 0 {
		return
	}
	next := r.queue[0]
	r.queue[0] = pending{}
	r.queue = r.queue[1:]
	r.queued.Set(now, float64(len(r.queue)))
	r.grant(next.since, next.start)
}

// ResetStats descarta as métricas acumuladas até agora, sem mexer em
// quem está sendo atendido ou esperando. Serve para descartar o warm-up.
func (r *Resource) ResetStats() {
	now := r.e.Now()
	r.busy.Reset(now)
	r.queued.Reset(now)
	r.waits = stats.Series{}
}

// Name devolve o nome do recurso.
func (r *Resource) Name() string { return r.name }

// Capacity devolve quantos servidores o recurso tem.
func (r *Resource) Capacity() int { return r.capacity }

// InUse devolve quantos servidores estão ocupados agora.
func (r *Resource) InUse() int { return r.inUse }

// QueueLen devolve quantos estão esperando agora.
func (r *Resource) QueueLen() int { return len(r.queue) }

// Utilization devolve a fração do tempo em que os servidores estiveram
// ocupados, ponderada pelo tempo: área sob a curva de ocupação dividida
// pela capacidade e pela duração da janela. Não é atendidos ÷ chegados.
func (r *Resource) Utilization() float64 {
	return r.busy.Mean(r.e.Now()) / float64(r.capacity)
}

// MeanQueue devolve o tamanho médio da fila, ponderado pelo tempo.
func (r *Resource) MeanQueue() float64 { return r.queued.Mean(r.e.Now()) }

// MaxQueue devolve o maior tamanho que a fila atingiu.
func (r *Resource) MaxQueue() int { return int(r.queued.Max()) }

// Waits devolve as esperas na fila de cada atendimento iniciado.
func (r *Resource) Waits() *stats.Series { return &r.waits }
