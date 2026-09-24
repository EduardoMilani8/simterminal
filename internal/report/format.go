// Package report formata resultados de cenários para gente ler (tabela
// no terminal) e para planilha (CSV).
package report

import (
	"fmt"
	"math"

	"github.com/eduardomilani8/simterminal/internal/stats"
)

// Duration formata minutos como "2h47" ou "38min". Arredonda ao minuto:
// com intervalo de confiança de ±10 min, segundo é teatro.
func Duration(min float64) string {
	m := int(math.Round(math.Abs(min)))
	sign := ""
	if min < 0 && m > 0 {
		sign = "−"
	}
	if m < 60 {
		return fmt.Sprintf("%s%dmin", sign, m)
	}
	return fmt.Sprintf("%s%dh%02d", sign, m/60, m%60)
}

// DurationCI formata um intervalo de tempo: "2h47 ± 11min".
func DurationCI(iv stats.Interval) string {
	if iv.N < 2 || math.Round(iv.HalfWidth) == 0 {
		return Duration(iv.Mean)
	}
	return Duration(iv.Mean) + " ± " + Duration(iv.HalfWidth)
}

// SignedDurationCI formata uma diferença de tempo com sinal explícito:
// "−1h49 ± 9min", "+3min ± 2min".
func SignedDurationCI(iv stats.Interval) string {
	s := Duration(iv.Mean)
	if math.Round(iv.Mean) > 0 {
		s = "+" + s
	}
	if iv.N < 2 || math.Round(iv.HalfWidth) == 0 {
		return s
	}
	return s + " ± " + Duration(iv.HalfWidth)
}

// Percent formata uma fração como porcentagem inteira: 0.964 → "96%".
func Percent(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }

// PercentCI formata uma fração com intervalo: "96% ± 1%".
func PercentCI(iv stats.Interval) string {
	if iv.N < 2 || math.Round(iv.HalfWidth*100) == 0 {
		return Percent(iv.Mean)
	}
	return Percent(iv.Mean) + " ± " + Percent(iv.HalfWidth)
}

// CountCI formata uma contagem com intervalo: "46 ± 3".
func CountCI(iv stats.Interval) string {
	if iv.N < 2 || math.Round(iv.HalfWidth) == 0 {
		return fmt.Sprintf("%.0f", iv.Mean)
	}
	return fmt.Sprintf("%.0f ± %.0f", iv.Mean, iv.HalfWidth)
}
