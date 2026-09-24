package porto

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eduardomilani8/simterminal/internal/antaq"
)

var origem = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func t(h float64) time.Time { return origem.Add(time.Duration(h * float64(time.Hour))) }

func atr(id, berco string, chegada, atracacao, desatracacao float64) antaq.Atracacao {
	return antaq.Atracacao{ID: id, Berco: berco, Chegada: t(chegada), Atracacao: t(atracacao), Desatracacao: t(desatracacao)}
}

func periodo(fimH float64) Janela {
	return Janela{Inicio: Data{origem}, Fim: Data{t(fimH)}}
}

// Um berço, cinco navios, com tudo o que complica: um navio já atracado
// na origem, fila, ultrapassagem (C chega antes de B e atraca depois) e
// um navio que atraca com o berço vazio há horas (não estava pronto).
func grupoPequeno() *DadosGrupo {
	as := []antaq.Atracacao{
		atr("0", "B1", -30, -20, 10),  // no berço na origem
		atr("A", "B1", 0, 12, 40),     // espera o 0 sair; 2 h de manobra
		atr("C", "B1", 5, 90, 120),    // ultrapassado por B
		atr("B", "B1", 20, 42, 80),    // 2 h de manobra depois do A
		atr("D", "B1", 130, 150, 170), // chega com o berço vazio e demora 20 h: não estava pronto
	}
	return MontarGrupo(Grupo{Nome: "g", Bercos: []string{"B1"}}, as, periodo(200))
}

func TestReplayReproduzOReal(t *testing.T) {
	checar := func(t *testing.T, d *DadosGrupo) {
		for _, modo := range []Prontidao{Otimista, Conservadora} {
			r := Replay{Dados: d, Cenario: Base(), Modo: modo}.Rodar()
			for i, n := range d.Navios {
				if math.Abs(r.Atracou[i]-n.Atracacao) > 1e-6 || math.Abs(r.Saiu[i]-n.Desatracacao) > 1e-6 {
					t.Fatalf("%s, navio %s: replay atracou %.3f saiu %.3f; real %.3f e %.3f",
						modo, n.ID, r.Atracou[i], r.Saiu[i], n.Atracacao, n.Desatracacao)
				}
			}
			if got, want := r.Resumo(), ResumoReal(d); math.Abs(got.EsperaMedia-want.EsperaMedia) > 1e-6 ||
				math.Abs(got.Ocupacao-want.Ocupacao) > 1e-9 {
				t.Fatalf("%s: resumo do replay %+v difere do real %+v", modo, got, want)
			}
		}
	}

	t.Run("sintético", func(t *testing.T) { checar(t, grupoPequeno()) })

	t.Run("píer com duas posições", func(t *testing.T) {
		as := []antaq.Atracacao{
			atr("1", "P", 0, 1, 50),
			atr("2", "P", 0, 3, 20), // lado de lá do píer
			atr("3", "P", 2, 22, 40),
			atr("4", "P", 10, 41, 60),
		}
		d := MontarGrupo(Grupo{Nome: "p", Bercos: []string{"P"}}, as, periodo(100))
		if d.Posicoes["P"] != 2 {
			t.Fatalf("posições = %d, esperava 2", d.Posicoes["P"])
		}
		checar(t, d)
	})

	t.Run("Paranaguá 2024", func(t *testing.T) {
		for _, d := range paranagua(t) {
			t.Run(d.Grupo.Nome, func(t *testing.T) { checar(t, d) })
		}
	})
}

func TestProntidaoEManobra(t *testing.T) {
	d := grupoPequeno()
	por := map[string]*Navio{}
	for i := range d.Navios {
		por[d.Navios[i].ID] = &d.Navios[i]
	}
	// intervalos com fila: 0→A 2 h, A→B 2 h, B→C 10 h (C chegou há muito);
	// mediana 2 h, percentil 90 10 h
	if got := d.ManobraTipica[Otimista]; got != 2 {
		t.Errorf("manobra típica = %v, esperava 2", got)
	}
	if got := d.LimiteManobra[Otimista]; got != 10 {
		t.Errorf("limite de manobra = %v, esperava 10", got)
	}
	// C: o berço liberou às 80 e C atracou às 90; 10 h cabe no limite,
	// então é manobra e C estava pronto ao chegar
	if c := por["C"]; c.Pronto[Otimista] != 5 || c.Manobra[Otimista] != 10 {
		t.Errorf("C otimista: pronto %v manobra %v; esperava 5 e 10", c.Pronto[Otimista], c.Manobra[Otimista])
	}
	// D: chegou às 130 com o berço vazio desde 120 e atracou às 150; 20 h
	// passa do limite: não estava pronto, fica pronto 2 h antes de atracar
	if dd := por["D"]; dd.Pronto[Otimista] != 148 || dd.Manobra[Otimista] != 2 {
		t.Errorf("D: pronto %v manobra %v; esperava 148 e 2", dd.Pronto[Otimista], dd.Manobra[Otimista])
	}
	// B: fila normal, pronto ao chegar
	if b := por["B"]; b.Pronto[Otimista] != 20 || b.Manobra[Otimista] != 2 {
		t.Errorf("B: pronto %v manobra %v; esperava 20 e 2", b.Pronto[Otimista], b.Manobra[Otimista])
	}
}

func TestBercoExtraNaoPiora(t *testing.T) {
	for _, d := range append(paranagua(t), grupoPequeno()) {
		for _, modo := range []Prontidao{Otimista, Conservadora} {
			comp := Cenario{Nome: "c", Alocacao: AlocCompartilhada, Ordem: OrdemReal}
			mais := comp
			mais.BercosExtra = 1
			a := Replay{Dados: d, Cenario: comp, Modo: modo}.Rodar().Resumo()
			b := Replay{Dados: d, Cenario: mais, Modo: modo}.Rodar().Resumo()
			if b.EsperaMedia > a.EsperaMedia+1e-9 {
				t.Errorf("%s %s: berço extra piorou a espera (%.1f → %.1f h)", d.Grupo.Nome, modo, a.EsperaMedia, b.EsperaMedia)
			}
		}
	}
}

func TestOperacaoMaisRapidaNaoPiora(t *testing.T) {
	for _, d := range append(paranagua(t), grupoPequeno()) {
		base := Replay{Dados: d, Cenario: Base()}.Rodar().Resumo()
		sc := Base()
		sc.Operacao = Fator(0.8)
		rapida := Replay{Dados: d, Cenario: sc}.Rodar().Resumo()
		if rapida.EsperaMedia > base.EsperaMedia+1e-9 {
			t.Errorf("%s: operar 20%% mais rápido piorou a espera (%.1f → %.1f h)", d.Grupo.Nome, base.EsperaMedia, rapida.EsperaMedia)
		}
	}
}

// paranagua carrega os grupos de exemplos/portos/paranagua.yaml, se os
// dados tiverem sido baixados. Sem eles, o teste é pulado: CI não baixa
// 100 MB.
func paranagua(t *testing.T) []*DadosGrupo {
	t.Helper()
	cfgPath := filepath.Join("..", "..", "portos", "paranagua.yaml")
	cfg, err := Carregar(cfgPath)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.PastaDados(), "2024Atracacao.txt")); err != nil {
		t.Skip("dados da ANTAQ não baixados (simterminal porto baixar)")
	}
	gs, err := CarregarGrupos(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	return gs
}
