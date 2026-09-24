package des

import "math"

// Exponential sorteia um intervalo com média mean. É a distribuição dos
// intervalos entre chegadas quando os caminhões chegam de forma
// independente uns dos outros (processo de Poisson).
func (e *Engine) Exponential(mean float64) float64 {
	return e.rng.ExpFloat64() * mean
}

// Uniform sorteia um valor entre min e max, com a mesma chance para
// qualquer ponto do intervalo.
func (e *Engine) Uniform(min, max float64) float64 {
	return min + e.rng.Float64()*(max-min)
}

// Triangular sorteia entre min e max com pico em mode. Boa escolha
// quando não há dado histórico, só a percepção de quem opera:
// "leva uns 40 minutos, no melhor caso 25, no pior 90".
func (e *Engine) Triangular(min, mode, max float64) float64 {
	u := e.rng.Float64()
	c := (mode - min) / (max - min)
	if u < c {
		return min + math.Sqrt(u*(max-min)*(mode-min))
	}
	return max - math.Sqrt((1-u)*(max-min)*(max-mode))
}
