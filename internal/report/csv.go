package report

import (
	"encoding/csv"
	"io"
	"sort"
	"strconv"

	"github.com/eduardomilani8/simterminal/internal/scenario"
	"github.com/eduardomilani8/simterminal/internal/stats"
	"github.com/eduardomilani8/simterminal/internal/terminal"
)

// WriteCSV escreve os resumos em formato longo — uma linha por cenário,
// recurso e métrica — que qualquer planilha filtra e tabela dinâmica
// sem sofrer. Tempos em minutos, frações entre 0 e 1.
func WriteCSV(w io.Writer, ss []*scenario.Summary) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"cenario", "recurso", "metrica", "media", "ic95", "replicas"})
	row := func(s *scenario.Summary, recurso, metrica string, iv stats.Interval) {
		cw.Write([]string{s.Name, recurso, metrica, num(iv.Mean), num(iv.HalfWidth), strconv.Itoa(iv.N)})
	}
	for _, s := range ss {
		row(s, "", "tempo_medio_min", s.MeanTime)
		row(s, "", "tempo_p95_min", s.P95Time)
		row(s, "", "fila_max_patio", s.MaxQueue)
		row(s, "", "caminhoes_atendidos", s.Trucks)
		row(s, "", "fim_operacao_min", s.EndTime)
		for _, r := range s.Resources {
			row(s, r.Name, "capacidade", stats.Interval{Mean: float64(r.Capacity), N: len(s.Seeds)})
			row(s, r.Name, "carga_oferecida", stats.Interval{Mean: r.OfferedLoad, N: len(s.Seeds)})
			row(s, r.Name, "utilizacao", r.Utilization)
			row(s, r.Name, "espera_media_min", r.MeanWait)
			row(s, r.Name, "fila_max", r.MaxQueue)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteTrucksCSV escreve a jornada de cada caminhão de uma réplica, com
// todos os marcos: é onde se descobre onde cada um perdeu o dia.
func WriteTrucksCSV(w io.Writer, r *terminal.Result) error {
	marks := []struct{ mark, column string }{
		{terminal.MarkArrival, "chegada"},
		{terminal.MarkWeighInStart, "balanca_entrada_inicio"},
		{terminal.MarkWeighInEnd, "balanca_entrada_fim"},
		{terminal.MarkUnloadStart, "descarga_inicio"},
		{terminal.MarkUnloadEnd, "descarga_fim"},
		{terminal.MarkWeighOutStart, "balanca_saida_inicio"},
		{terminal.MarkExit, "saida"},
	}
	cw := csv.NewWriter(w)
	header := []string{"id", "tipo_carga", "agendado"}
	for _, m := range marks {
		header = append(header, m.column+"_min")
	}
	cw.Write(append(header, "tempo_total_min"))

	trucks := append([]*terminal.Caminhao(nil), r.Trucks...)
	sort.Slice(trucks, func(i, j int) bool { return trucks[i].ID < trucks[j].ID })
	for _, c := range trucks {
		rec := []string{strconv.Itoa(c.ID), c.TipoCarga, strconv.FormatBool(c.Agendado)}
		for _, m := range marks {
			rec = append(rec, num(c.Marcos[m.mark]))
		}
		cw.Write(append(rec, num(c.TimeInSystem())))
	}
	cw.Flush()
	return cw.Error()
}

func num(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }
