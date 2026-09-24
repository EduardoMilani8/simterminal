package stats

// TimeWeighted acumula a média no tempo de uma grandeza que muda em
// saltos: servidores ocupados, tamanho da fila, caminhões no pátio.
//
// Não é a média dos valores registrados — é a área sob a curva dividida
// pelo tempo. Uma fila de 30 caminhões que durou um minuto pesa bem
// menos que uma de 5 que durou a tarde toda.
//
// O valor zero começa em t=0 com valor 0.
type TimeWeighted struct {
	start float64 // início da janela de medição
	last  float64 // instante da última mudança
	value float64 // valor atual
	area  float64 // integral de value desde start até last
	max   float64 // maior valor visto desde start
}

// Set registra que a grandeza passou a valer v no instante t.
func (w *TimeWeighted) Set(t, v float64) {
	w.area += w.value * (t - w.last)
	w.last = t
	w.value = v
	if v > w.max {
		w.max = v
	}
}

// Add soma delta ao valor atual no instante t.
func (w *TimeWeighted) Add(t, delta float64) { w.Set(t, w.value+delta) }

// Reset descarta tudo o que foi acumulado e reabre a janela em t,
// mantendo o valor atual. É assim que se descarta o warm-up.
func (w *TimeWeighted) Reset(t float64) {
	w.start = t
	w.last = t
	w.area = 0
	w.max = w.value
}

// Value devolve o valor atual.
func (w *TimeWeighted) Value() float64 { return w.value }

// Max devolve o maior valor visto desde o início da janela.
func (w *TimeWeighted) Max() float64 { return w.max }

// Area devolve a integral da grandeza desde o início da janela até t.
func (w *TimeWeighted) Area(t float64) float64 {
	return w.area + w.value*(t-w.last)
}

// Mean devolve a média no tempo desde o início da janela até t.
func (w *TimeWeighted) Mean(t float64) float64 {
	d := t - w.start
	if d <= 0 {
		return w.value
	}
	return w.Area(t) / d
}
