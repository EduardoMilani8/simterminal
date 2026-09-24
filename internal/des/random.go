package des

import (
	"hash/fnv"
	"math"
	"math/rand"
)

// Stream é um gerador de números aleatórios com as distribuições usadas
// na simulação.
type Stream struct {
	rng *rand.Rand
}

// NewStream cria o fluxo identificado por name a partir de seed.
func NewStream(seed int64, name string) *Stream {
	h := fnv.New64a()
	h.Write([]byte(name))
	mixed := splitmix64(uint64(seed) ^ h.Sum64())
	return &Stream{rng: rand.New(rand.NewSource(int64(mixed)))}
}

// splitmix64 espalha os bits da semente, para que sementes vizinhas
// (1, 2, 3...) gerem fluxos sem correlação entre si.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// Float64 sorteia um valor em [0, 1).
func (s *Stream) Float64() float64 { return s.rng.Float64() }

// Exponential sorteia um intervalo com média mean. É a distribuição dos
// intervalos entre chegadas quando os caminhões chegam de forma
// independente uns dos outros (processo de Poisson).
func (s *Stream) Exponential(mean float64) float64 {
	return s.rng.ExpFloat64() * mean
}

// Uniform sorteia um valor entre min e max, com a mesma chance para
// qualquer ponto do intervalo.
func (s *Stream) Uniform(min, max float64) float64 {
	return min + s.rng.Float64()*(max-min)
}

// Triangular sorteia entre min e max com pico em mode. Boa escolha
// quando não há dado histórico, só a percepção de quem opera:
// "leva uns 40 minutos, no melhor caso 25, no pior 90".
func (s *Stream) Triangular(min, mode, max float64) float64 {
	if max == min {
		return min
	}
	u := s.rng.Float64()
	c := (mode - min) / (max - min)
	if u < c {
		return min + math.Sqrt(u*(max-min)*(mode-min))
	}
	return max - math.Sqrt((1-u)*(max-min)*(max-mode))
}

// NextArrival sorteia o próximo instante de chegada depois de now num
// processo de Poisson não-homogêneo com taxa rate(t), limitada por
// maxRate. Devolve false se não houver chegada antes de until.
//
// Usa thinning: sorteia candidatos na taxa máxima e aceita cada um com
// probabilidade rate(t)/maxRate. Trocar a média a cada hora distorce o
// comportamento justamente nas viradas de hora, onde está o pico.
func (s *Stream) NextArrival(now float64, rate func(t float64) float64, maxRate, until float64) (float64, bool) {
	if maxRate <= 0 {
		return 0, false
	}
	t := now
	for {
		t += s.rng.ExpFloat64() / maxRate
		if t >= until {
			return 0, false
		}
		if s.rng.Float64()*maxRate < rate(t) {
			return t, true
		}
	}
}

// Exponential sorteia do fluxo principal do motor. Veja Stream.Exponential.
func (e *Engine) Exponential(mean float64) float64 { return e.main.Exponential(mean) }

// Uniform sorteia do fluxo principal do motor. Veja Stream.Uniform.
func (e *Engine) Uniform(min, max float64) float64 { return e.main.Uniform(min, max) }

// Triangular sorteia do fluxo principal do motor. Veja Stream.Triangular.
func (e *Engine) Triangular(min, mode, max float64) float64 {
	return e.main.Triangular(min, mode, max)
}
