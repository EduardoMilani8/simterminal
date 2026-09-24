package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// portoSintetico escreve, no formato da ANTAQ, um porto de dois berços
// com fila por ordem de chegada, de 2022 a meados de 2025, e o YAML dele.
func portoSintetico(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dados := filepath.Join(dir, "dados")
	if err := os.MkdirAll(dados, 0o755); err != nil {
		t.Fatal(err)
	}
	const cab = "\ufeffIDAtracacao;CDTUP;IDBerco;Berço;Porto Atracação;Terminal;Data Chegada;Data Atracação;Data Início Operação;Data Término Operação;Data Desatracação;Tipo de Operação;Tipo de Navegação da Atracação;Nº do IMO\r\n"
	const cabT = "\ufeffIDAtracacao;TEsperaAtracacao;TEsperaInicioOp;TOperacao;TEsperaDesatracacao;TAtracado;TEstadia\r\n"
	atr := map[int]*strings.Builder{}
	tmp := map[int]*strings.Builder{}
	for a := 2022; a <= 2025; a++ {
		atr[a], tmp[a] = &strings.Builder{}, &strings.Builder{}
		atr[a].WriteString(cab)
		tmp[a].WriteString(cabT)
	}
	f := func(t time.Time) string { return t.Format("02/01/2006 15:04:05") }

	rng := rand.New(rand.NewSource(1))
	h := func(x float64) time.Duration { return time.Duration(x * float64(time.Hour)) }
	chegada := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	fim := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	livre := []time.Time{chegada, chegada}
	for id := 1; ; id++ {
		chegada = chegada.Add(h(rng.ExpFloat64() * 30))
		if chegada.After(fim) {
			break
		}
		b := 0
		if livre[1].Before(livre[0]) {
			b = 1
		}
		atraca := chegada
		if livre[b].After(chegada) {
			atraca = livre[b]
		}
		atraca = atraca.Add(h(2))
		ini := atraca.Add(h(2))
		term := ini.Add(h(40 + 30*rng.Float64()))
		desat := term.Add(h(3))
		livre[b] = desat
		a := desat.Year()
		if a > 2025 {
			break
		}
		fmt.Fprintf(atr[a], "%d;BRXXX;XX0%d;Berço %d;Porto Teste;Terminal;%s;%s;%s;%s;%s;Movimentação da Carga;Longo Curso;%d\r\n",
			id, b+1, b+1, f(chegada), f(atraca), f(ini), f(term), f(desat), 9000000+id)
		fmt.Fprintf(tmp[a], "%d;%s;2;%s;3;%s;%s\r\n", id,
			strings.Replace(fmt.Sprintf("%.2f", atraca.Sub(chegada).Hours()), ".", ",", 1),
			strings.Replace(fmt.Sprintf("%.2f", term.Sub(ini).Hours()), ".", ",", 1),
			strings.Replace(fmt.Sprintf("%.2f", desat.Sub(atraca).Hours()), ".", ",", 1),
			strings.Replace(fmt.Sprintf("%.2f", desat.Sub(chegada).Hours()), ".", ",", 1))
	}
	for a := 2022; a <= 2025; a++ {
		os.WriteFile(filepath.Join(dados, fmt.Sprintf("%dAtracacao.txt", a)), []byte(atr[a].String()), 0o644)
		os.WriteFile(filepath.Join(dados, fmt.Sprintf("%dTemposAtracacao.txt", a)), []byte(tmp[a].String()), 0o644)
	}
	yaml := `porto: Porto Teste
dados: dados
anos: [2022, 2023, 2024, 2025]
periodo: {inicio: 2024-01-01, fim: 2025-01-01}
treino:  {inicio: 2023-01-01, fim: 2024-01-01}
custo_navio_dia_usd: 20000
grupos:
  - nome: Cais
    bercos: [XX01, XX02]
cenarios:
  - nome: + 1 berço
    alocacao: compartilhada
    bercos_extra: 1
`
	p := filepath.Join(dir, "teste.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCLIPorto(t *testing.T) {
	cfg := portoSintetico(t)
	casos := []struct {
		args []string
		want []string
	}{
		{[]string{"diagnostico", "-c", cfg}, []string{"Porto Teste — navios que chegaram em 2024", "Cais", "US$"}},
		{[]string{"cenarios", cfg}, []string{"real", "+ 1 berço", "custo evitado"}},
		{[]string{"capacidade", "-c", cfg, "-meta", "2d"}, []string{"como opera hoje", "← hoje"}},
		{[]string{"previsao", "-c", cfg}, []string{"fila ÷ vazão recente", "← escolhido no treino"}},
		{[]string{"chegada", "-c", cfg, "-grupo", "Cais"}, []string{"oráculo", "fila virtual: reserva que cobre as 48 h"}},
		{[]string{"bercos", "-c", cfg}, []string{"XX01", "XX02", "Cais"}},
	}
	for _, c := range casos {
		code, out, errs := runCLI(t, append([]string{"porto"}, c.args...)...)
		if code != 0 {
			t.Fatalf("porto %v saiu com %d: %s", c.args, code, errs)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("porto %v: saída sem %q:\n%s", c.args, w, out)
			}
		}
	}
}

func TestCLIPortoErros(t *testing.T) {
	cfg := portoSintetico(t)
	casos := []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"porto"}, 2, "simterminal porto baixar"},
		{[]string{"porto", "voar"}, 2, "comando desconhecido"},
		{[]string{"porto", "diagnostico"}, 2, "informe o porto"},
		{[]string{"porto", "cenarios", "-c", cfg, "-grupo", "Nada"}, 1, "os grupos são: Cais"},
		{[]string{"porto", "capacidade", "-c", cfg, "-meta", "logo"}, 1, "meta inválida"},
	}
	for _, c := range casos {
		code, _, errs := runCLI(t, c.args...)
		if code != c.code || !strings.Contains(errs, c.msg) {
			t.Errorf("%v: código %d, stderr %q; esperava %d e %q", c.args, code, errs, c.code, c.msg)
		}
	}

	// sem os dados: a mensagem diz como baixar
	os.RemoveAll(filepath.Join(filepath.Dir(cfg), "dados"))
	code, _, errs := runCLI(t, "porto", "diagnostico", "-c", cfg)
	if code != 1 || !strings.Contains(errs, "porto baixar") {
		t.Errorf("sem dados: código %d, stderr %q", code, errs)
	}
}
