package terminal

import (
	"errors"
	"fmt"
	"math"
)

// Triangular descreve um tempo de serviço do jeito que o operador sabe
// dizer: "leva uns 48 minutos, no melhor caso 35, no pior 95".
type Triangular struct {
	Min  float64 `yaml:"min"`
	Mode float64 `yaml:"moda"`
	Max  float64 `yaml:"max"`
}

// Mean devolve a média da distribuição.
func (d Triangular) Mean() float64 { return (d.Min + d.Mode + d.Max) / 3 }

func (d Triangular) validate(what string) error {
	if d.Min < 0 || d.Min > d.Mode || d.Mode > d.Max {
		return fmt.Errorf("%s: precisa de 0 <= min <= moda <= max (veio min=%g moda=%g max=%g)",
			what, d.Min, d.Mode, d.Max)
	}
	if d.Max == 0 {
		return fmt.Errorf("%s: tempo zerado", what)
	}
	return nil
}

// Cargo é um tipo de carga, com seu tempo de descarga e a fração dos
// caminhões que o trazem.
type Cargo struct {
	Name   string
	Unload Triangular
	Share  float64
}

// Config descreve um terminal e a demanda sobre ele. Tempos em minutos.
type Config struct {
	// Horizon é por quanto tempo o portão recebe caminhões. Quem entrou
	// até lá é atendido até o fim, mesmo que passe do horizonte.
	Horizon float64
	// Warmup é o período inicial que não entra na estatística: o terminal
	// começa vazio, o que nenhum terminal real está.
	Warmup float64
	// HourlyProfile é a taxa de chegada em caminhões por hora, uma posição
	// por hora a partir de t=0. Fora do perfil não chega ninguém.
	HourlyProfile []float64

	EntryScales int // balanças de entrada
	ExitScales  int // balanças de saída; 0 = divide as de entrada
	Docks       int

	Weighing Triangular // tempo de uma pesagem
	Cargo    []Cargo

	// Scheduled é a fração dos caminhões que vem com hora marcada. Os
	// agendados são distribuídos em horários igualmente espaçados ao longo
	// do horizonte; os demais seguem o perfil horário.
	Scheduled float64
	// ScheduleTolerance é o quanto um agendado pode se adiantar ou atrasar
	// em relação ao horário marcado, em minutos.
	ScheduleTolerance float64
}

// Validate aponta o primeiro problema da configuração.
func (c *Config) Validate() error {
	switch {
	case c.Horizon <= 0:
		return errors.New("horizonte precisa ser positivo")
	case c.Warmup < 0 || c.Warmup >= c.Horizon:
		return fmt.Errorf("warm-up (%g min) precisa ficar entre 0 e o horizonte (%g min)", c.Warmup, c.Horizon)
	case len(c.HourlyProfile) == 0:
		return errors.New("perfil horário de chegadas vazio")
	case c.EntryScales < 1:
		return errors.New("precisa de pelo menos uma balança de entrada")
	case c.ExitScales < 0:
		return errors.New("número de balanças de saída negativo")
	case c.Docks < 1:
		return errors.New("precisa de pelo menos uma doca")
	case len(c.Cargo) == 0:
		return errors.New("nenhum tipo de carga configurado")
	case c.Scheduled < 0 || c.Scheduled > 1:
		return fmt.Errorf("fração agendada %g fora de [0, 1]", c.Scheduled)
	case c.ScheduleTolerance < 0:
		return errors.New("tolerância do agendamento negativa")
	}
	total := 0.0
	for h, v := range c.HourlyProfile {
		if v < 0 || math.IsNaN(v) {
			return fmt.Errorf("perfil horário: hora %d com taxa %g", h, v)
		}
		total += v
	}
	if total == 0 {
		return errors.New("perfil horário sem nenhuma chegada")
	}
	if err := c.Weighing.validate("pesagem"); err != nil {
		return err
	}
	shares := 0.0
	for _, cg := range c.Cargo {
		if err := cg.Unload.validate("descarga " + cg.Name); err != nil {
			return err
		}
		if cg.Share <= 0 {
			return fmt.Errorf("descarga %s: proporção precisa ser positiva", cg.Name)
		}
		shares += cg.Share
	}
	if math.Abs(shares-1) > 1e-6 {
		return fmt.Errorf("proporções dos tipos de carga somam %g, precisam somar 1", shares)
	}
	return nil
}

// rate devolve a taxa de chegada do perfil horário no instante t, em
// caminhões por minuto.
func (c *Config) rate(t float64) float64 {
	h := int(t / 60)
	if t < 0 || h >= len(c.HourlyProfile) {
		return 0
	}
	return c.HourlyProfile[h] / 60
}

// ExpectedArrivals devolve quantos caminhões o perfil manda, em média,
// dentro do horizonte.
func (c *Config) ExpectedArrivals() float64 {
	total := 0.0
	for h, v := range c.HourlyProfile {
		start := float64(h) * 60
		if start >= c.Horizon {
			break
		}
		total += v * math.Min(60, c.Horizon-start) / 60
	}
	return total
}

// MeanUnload devolve o tempo médio de descarga, ponderado pelo mix.
func (c *Config) MeanUnload() float64 {
	m := 0.0
	for _, cg := range c.Cargo {
		m += cg.Share * cg.Unload.Mean()
	}
	return m
}

// OfferedLoad devolve, para cada recurso, a carga oferecida média ao
// longo do horizonte: trabalho que chega ÷ capacidade disponível. Acima
// de 1 a fila cresce sem parar, e a "média" passa a depender do
// horizonte escolhido, não do sistema.
func (c *Config) OfferedLoad() map[string]float64 {
	n := c.ExpectedArrivals()
	w := c.Weighing.Mean()
	load := map[string]float64{
		ResDocks: n * c.MeanUnload() / (float64(c.Docks) * c.Horizon),
	}
	if c.ExitScales == 0 {
		load[ResSharedScale] = 2 * n * w / (float64(c.EntryScales) * c.Horizon)
	} else {
		load[ResEntryScale] = n * w / (float64(c.EntryScales) * c.Horizon)
		load[ResExitScale] = n * w / (float64(c.ExitScales) * c.Horizon)
	}
	return load
}
