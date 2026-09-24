package scenario

import (
	"github.com/eduardomilani8/simterminal/internal/stats"
	"github.com/eduardomilani8/simterminal/internal/terminal"
)

// ResourceSummary agrega um recurso ao longo das réplicas.
type ResourceSummary struct {
	Name        string
	Capacity    int
	OfferedLoad float64
	Utilization stats.Interval
	MeanWait    stats.Interval // minutos
	MaxQueue    stats.Interval
}

// Summary é a resposta de um cenário: cada número vem com seu intervalo
// de confiança de 95% calculado entre réplicas.
type Summary struct {
	Name     string
	Config   terminal.Config
	Seeds    []int64
	Results  []terminal.Result
	MeanTime stats.Interval // tempo médio no terminal, minutos
	P95Time  stats.Interval // p95 do tempo no terminal, minutos
	MaxQueue stats.Interval // fila máxima no pátio (antes da balança de entrada)
	Trucks   stats.Interval // caminhões atendidos por réplica
	EndTime  stats.Interval // quando o último caminhão sai, minutos

	Resources []ResourceSummary
	// Bottleneck é o recurso onde os caminhões mais perdem tempo na fila;
	// vazio se nenhum faz esperar de forma relevante.
	Bottleneck string
	// Saturated lista os recursos com carga oferecida >= 1: a fila cresce
	// sem parar e as médias dependem do horizonte, não do sistema.
	Saturated []string
}

// Per extrai uma métrica de cada réplica, na ordem das sementes.
func (s *Summary) Per(metric func(*terminal.Result) float64) []float64 {
	xs := make([]float64, len(s.Results))
	for i := range s.Results {
		xs[i] = metric(&s.Results[i])
	}
	return xs
}

// Métricas por réplica usadas no resumo e nas comparações.
var (
	MetricMeanTime = func(r *terminal.Result) float64 { return r.TimeInSystem.Mean() }
	MetricP95Time  = func(r *terminal.Result) float64 { return r.TimeInSystem.Percentile(95) }
	MetricMaxQueue = func(r *terminal.Result) float64 { return float64(r.Resources[0].MaxQueue) }
)

// minRelevantWait é a espera média, em minutos, abaixo da qual nenhum
// recurso é apontado como gargalo.
const minRelevantWait = 1.0

// Run roda o cenário com as sementes dadas e resume o resultado.
func Run(name string, cfg terminal.Config, seeds []int64) Summary {
	return Summarize(name, cfg, seeds, RunReplicas(cfg, seeds))
}

// Summarize agrega réplicas já rodadas.
func Summarize(name string, cfg terminal.Config, seeds []int64, results []terminal.Result) Summary {
	s := Summary{Name: name, Config: cfg, Seeds: seeds, Results: results}
	s.MeanTime = stats.MeanCI(s.Per(MetricMeanTime))
	s.P95Time = stats.MeanCI(s.Per(MetricP95Time))
	s.MaxQueue = stats.MeanCI(s.Per(MetricMaxQueue))
	s.Trucks = stats.MeanCI(s.Per(func(r *terminal.Result) float64 { return float64(r.Completed) }))
	s.EndTime = stats.MeanCI(s.Per(func(r *terminal.Result) float64 { return r.EndTime }))

	if len(results) == 0 {
		return s
	}
	worstWait := minRelevantWait
	for i, rr := range results[0].Resources {
		i := i
		pick := func(f func(terminal.ResourceResult) float64) stats.Interval {
			return stats.MeanCI(s.Per(func(r *terminal.Result) float64 { return f(r.Resources[i]) }))
		}
		rs := ResourceSummary{
			Name:        rr.Name,
			Capacity:    rr.Capacity,
			OfferedLoad: rr.OfferedLoad,
			Utilization: pick(func(r terminal.ResourceResult) float64 { return r.Utilization }),
			MeanWait:    pick(func(r terminal.ResourceResult) float64 { return r.MeanWait }),
			MaxQueue:    pick(func(r terminal.ResourceResult) float64 { return float64(r.MaxQueue) }),
		}
		s.Resources = append(s.Resources, rs)

		if rs.MeanWait.Mean > worstWait {
			worstWait = rs.MeanWait.Mean
			s.Bottleneck = rs.Name
		}
		if rs.OfferedLoad >= 1 {
			s.Saturated = append(s.Saturated, rs.Name)
		}
	}
	return s
}

// Resource devolve o resumo do recurso com o nome dado.
func (s *Summary) Resource(name string) (ResourceSummary, bool) {
	for _, r := range s.Resources {
		if r.Name == name {
			return r, true
		}
	}
	return ResourceSummary{}, false
}

// Diff compara uma métrica entre dois cenários rodados com as mesmas
// sementes: média de (a − b) réplica a réplica, com IC pareado.
func Diff(a, b *Summary, metric func(*terminal.Result) float64) stats.Interval {
	return stats.PairedDiffCI(a.Per(metric), b.Per(metric))
}
