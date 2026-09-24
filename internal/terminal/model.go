package terminal

import (
	"math"
	"sort"

	"github.com/eduardomilani8/simterminal/internal/des"
	"github.com/eduardomilani8/simterminal/internal/stats"
)

// Nomes dos recursos do terminal.
const (
	ResEntryScale  = "balança de entrada"
	ResExitScale   = "balança de saída"
	ResSharedScale = "balança"
	ResDocks       = "docas"
)

// Marcos registrados na jornada de cada caminhão.
const (
	MarkArrival       = "chegada"
	MarkWeighInStart  = "balança entrada início"
	MarkWeighInEnd    = "balança entrada fim"
	MarkUnloadStart   = "descarga início"
	MarkUnloadEnd     = "descarga fim"
	MarkWeighOutStart = "balança saída início"
	MarkExit          = "saída"
)

// Caminhao é a entidade que flui pelo terminal. Os marcos permitem
// reconstruir depois onde exatamente cada um perdeu o dia.
type Caminhao struct {
	ID        int
	TipoCarga string
	Agendado  bool
	ChegadaEm float64
	Marcos    map[string]float64

	// tempos sorteados na chegada, de um fluxo próprio: assim o mesmo
	// caminhão demora o mesmo tanto em qualquer cenário
	pesagemEntrada, descarga, pesagemSaida float64
	medido                                 bool // chegou depois do warm-up
}

// TimeInSystem devolve quanto o caminhão ficou dentro do terminal.
func (c *Caminhao) TimeInSystem() float64 { return c.Marcos[MarkExit] - c.ChegadaEm }

// ResourceResult resume um recurso numa rodada.
type ResourceResult struct {
	Name        string
	Capacity    int
	Utilization float64 // ponderada pelo tempo, entre o warm-up e o horizonte
	MeanQueue   float64 // fila média no mesmo período
	MaxQueue    int     // maior fila depois do warm-up
	MeanWait    float64 // espera média na fila, por atendimento
	P95Wait     float64
	OfferedLoad float64 // carga oferecida pela configuração
}

// Result é o que uma rodada (uma semente) produz. Conta só os caminhões
// que chegaram depois do warm-up.
type Result struct {
	Seed         int64
	Arrived      int
	Completed    int
	TimeInSystem stats.Series // minutos, por caminhão
	// MeanInSystem é o número médio de caminhões dentro do terminal (L),
	// ponderado pelo tempo, do fim do warm-up até o último sair.
	MeanInSystem float64
	// EndTime é quando o último caminhão saiu.
	EndTime   float64
	Resources []ResourceResult
	Trucks    []*Caminhao // os medidos, na ordem de saída
}

// ArrivalRate devolve a taxa de chegada observada (λ) em caminhões por
// minuto, na mesma janela de MeanInSystem.
func (r *Result) ArrivalRate(warmup float64) float64 {
	d := r.EndTime - warmup
	if d <= 0 {
		return 0
	}
	return float64(r.Arrived) / d
}

// Resource devolve o resultado do recurso com o nome dado.
func (r *Result) Resource(name string) (ResourceResult, bool) {
	for _, rr := range r.Resources {
		if rr.Name == name {
			return rr, true
		}
	}
	return ResourceResult{}, false
}

type model struct {
	cfg Config
	e   *des.Engine

	weighIn, docks, weighOut *des.Resource
	resources                []*des.Resource

	chegadas, atributos, agenda *des.Stream

	measuring bool
	inSystem  stats.TimeWeighted
	waits     map[*des.Resource]*stats.Series
	util      map[*des.Resource][2]float64 // utilização e fila média no horizonte
	nextID    int
	res       Result
}

// Run simula um dia do terminal com a semente dada.
func Run(cfg Config, seed int64) Result {
	e := des.New(seed)
	m := &model{
		cfg:       cfg,
		e:         e,
		chegadas:  e.Stream("chegadas"),
		atributos: e.Stream("atributos"),
		agenda:    e.Stream("agendamento"),
		measuring: cfg.Warmup <= 0,
		waits:     map[*des.Resource]*stats.Series{},
		util:      map[*des.Resource][2]float64{},
		res:       Result{Seed: seed},
	}

	if cfg.ExitScales == 0 {
		m.weighIn = des.NewResource(e, ResSharedScale, cfg.EntryScales)
		m.weighOut = m.weighIn
		m.docks = des.NewResource(e, ResDocks, cfg.Docks)
		m.resources = []*des.Resource{m.weighIn, m.docks}
	} else {
		m.weighIn = des.NewResource(e, ResEntryScale, cfg.EntryScales)
		m.docks = des.NewResource(e, ResDocks, cfg.Docks)
		m.weighOut = des.NewResource(e, ResExitScale, cfg.ExitScales)
		m.resources = []*des.Resource{m.weighIn, m.docks, m.weighOut}
	}
	for _, r := range m.resources {
		m.waits[r] = &stats.Series{}
	}

	if cfg.Warmup > 0 {
		e.ScheduleAt(cfg.Warmup, "fim do warm-up", m.endWarmup)
	}
	e.ScheduleAt(cfg.Horizon, "fim do expediente", m.snapshotUtilization)
	m.scheduleAppointments()
	m.scheduleNextArrival(0)

	e.Run(0)
	return m.finish()
}

func (m *model) endWarmup() {
	m.measuring = true
	m.inSystem.Reset(m.e.Now())
	for _, r := range m.resources {
		r.ResetStats()
	}
}

func (m *model) snapshotUtilization() {
	for _, r := range m.resources {
		m.util[r] = [2]float64{r.Utilization(), r.MeanQueue()}
	}
}

// scheduleNextArrival agenda a próxima chegada espontânea (sem hora
// marcada), seguindo o perfil horário reduzido pela fração agendada.
func (m *model) scheduleNextArrival(from float64) {
	free := 1 - m.cfg.Scheduled
	maxRate := 0.0
	for _, v := range m.cfg.HourlyProfile {
		maxRate = math.Max(maxRate, v/60*free)
	}
	rate := func(t float64) float64 { return m.cfg.rate(t) * free }

	t, ok := m.chegadas.NextArrival(from, rate, maxRate, m.cfg.Horizon)
	if !ok {
		return
	}
	m.e.ScheduleAt(t, "chegada", func() {
		m.arrive(false)
		m.scheduleNextArrival(t)
	})
}

// scheduleAppointments distribui os agendados em horários igualmente
// espaçados no horizonte, cada um chegando com uma folga sorteada em
// torno do horário marcado.
func (m *model) scheduleAppointments() {
	n := int(math.Round(m.cfg.Scheduled * m.cfg.ExpectedArrivals()))
	if n == 0 {
		return
	}
	times := make([]float64, n)
	tol := m.cfg.ScheduleTolerance
	for i := range times {
		slot := m.cfg.Horizon * (float64(i) + 0.5) / float64(n)
		t := slot + m.agenda.Uniform(-tol, tol)
		times[i] = math.Min(math.Max(t, 0), math.Nextafter(m.cfg.Horizon, 0))
	}
	sort.Float64s(times)
	for _, t := range times {
		m.e.ScheduleAt(t, "chegada agendada", func() { m.arrive(true) })
	}
}

func (m *model) newTruck(scheduled bool) *Caminhao {
	m.nextID++
	now := m.e.Now()
	c := &Caminhao{
		ID:        m.nextID,
		Agendado:  scheduled,
		ChegadaEm: now,
		Marcos:    map[string]float64{MarkArrival: now},
		medido:    m.measuring,
	}

	// sempre os mesmos quatro sorteios, na mesma ordem
	u := m.atributos.Float64()
	cargo := m.cfg.Cargo[len(m.cfg.Cargo)-1]
	acc := 0.0
	for _, cg := range m.cfg.Cargo {
		acc += cg.Share
		if u < acc {
			cargo = cg
			break
		}
	}
	w := m.cfg.Weighing
	c.TipoCarga = cargo.Name
	c.pesagemEntrada = m.atributos.Triangular(w.Min, w.Mode, w.Max)
	c.descarga = m.atributos.Triangular(cargo.Unload.Min, cargo.Unload.Mode, cargo.Unload.Max)
	c.pesagemSaida = m.atributos.Triangular(w.Min, w.Mode, w.Max)
	return c
}

// arrive é o fluxo do caminhão:
//
//	chegada → fila do pátio → balança entrada → doca → balança saída → saída
func (m *model) arrive(scheduled bool) {
	c := m.newTruck(scheduled)
	if c.medido {
		m.res.Arrived++
	}
	m.inSystem.Add(m.e.Now(), 1)

	m.use(m.weighIn, c, c.pesagemEntrada, MarkWeighInStart, MarkWeighInEnd, func() {
		m.use(m.docks, c, c.descarga, MarkUnloadStart, MarkUnloadEnd, func() {
			m.use(m.weighOut, c, c.pesagemSaida, MarkWeighOutStart, MarkExit, func() {
				m.exit(c)
			})
		})
	})
}

// use pede o recurso, segura por dur minutos, marca início e fim e
// segue para next.
func (m *model) use(r *des.Resource, c *Caminhao, dur float64, startMark, endMark string, next func()) {
	queuedAt := m.e.Now()
	r.Request(func(release func()) {
		now := m.e.Now()
		c.Marcos[startMark] = now
		if c.medido {
			m.waits[r].Add(now - queuedAt)
		}
		m.e.ScheduleIn(dur, endMark, func() {
			c.Marcos[endMark] = m.e.Now()
			release()
			next()
		})
	})
}

func (m *model) exit(c *Caminhao) {
	m.inSystem.Add(m.e.Now(), -1)
	if !c.medido {
		return
	}
	m.res.Completed++
	m.res.TimeInSystem.Add(c.TimeInSystem())
	m.res.Trucks = append(m.res.Trucks, c)
}

func (m *model) finish() Result {
	end := m.e.Now()
	m.res.EndTime = end
	m.res.MeanInSystem = m.inSystem.Mean(end)
	load := m.cfg.OfferedLoad()
	for _, r := range m.resources {
		u := m.util[r]
		m.res.Resources = append(m.res.Resources, ResourceResult{
			Name:        r.Name(),
			Capacity:    r.Capacity(),
			Utilization: u[0],
			MeanQueue:   u[1],
			MaxQueue:    r.MaxQueue(),
			MeanWait:    m.waits[r].Mean(),
			P95Wait:     m.waits[r].Percentile(95),
			OfferedLoad: load[r.Name()],
		})
	}
	return m.res
}
