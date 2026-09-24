package report

import "testing"

func TestFormatosPorto(t *testing.T) {
	casos := []struct{ got, want string }{
		{Dias(20), "20 h"},
		{Dias(47.4), "47 h"},
		{Dias(48), "2,0 d"},
		{Dias(285.6), "11,9 d"},
		{FaixaDias(200, 250), "8,3 a 10,4 d"},
		{FaixaDias(20, 30), "20 a 30 h"},
		{FaixaDias(20, 100), "20 h a 4,2 d"},
		{FaixaDias(100, 100.4), "4,2 d"},
		{Dias(-5), "−5 h"},
		{Dias(-0.3), "0 h"},
		{Milhar(4365.4), "4.365"},
		{Milhar(1234567), "1.234.567"},
		{Milhar(999), "999"},
		{Milhar(-1500), "−1.500"},
		{faixaPercent(-0.26, -0.02), "−26 a −2%"},
		{faixaPercent(-0.001, 0.001), "0%"},
	}
	for _, c := range casos {
		if c.got != c.want {
			t.Errorf("veio %q, esperava %q", c.got, c.want)
		}
	}
}
