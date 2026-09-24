package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLIRun(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "resumo.csv")
	trucks := filepath.Join(dir, "caminhoes.csv")

	code, out, errs := runCLI(t, "run", "-c", "../../exemplos/atual.yaml", "-replicas", "5",
		"-csv", csvPath, "-caminhoes", trucks)
	if code != 0 {
		t.Fatalf("saiu com %d: %s", code, errs)
	}
	for _, want := range []string{"atual — 5 réplicas (sementes 1–5)", "gargalo:", "resumo gravado"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída sem %q:\n%s", want, out)
		}
	}
	for _, p := range []string{csvPath, trucks} {
		if st, err := os.Stat(p); err != nil || st.Size() == 0 {
			t.Errorf("%s não foi gravado", p)
		}
	}
}

func TestCLICompareWithFlagsAfterFiles(t *testing.T) {
	code, out, errs := runCLI(t, "compare", "../../exemplos/atual.yaml", "../../exemplos/maisbalanca.yaml",
		"-replicas", "5", "-semente", "100")
	if code != 0 {
		t.Fatalf("saiu com %d: %s", code, errs)
	}
	for _, want := range []string{"2 cenários com as mesmas 5 réplicas (sementes 100–104)", "+ 1 balança", "Conclusão"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída sem %q:\n%s", want, out)
		}
	}
}

func TestCLIDemo(t *testing.T) {
	code, out, _ := runCLI(t, "demo")
	if code != 0 || !strings.Contains(out, "utilização da balança  72%") {
		t.Errorf("demo mudou (código %d):\n%s", code, out)
	}
}

func TestCLIErrors(t *testing.T) {
	cases := [][]string{
		{},
		{"voar"},
		{"run"},
		{"run", "-c", "nao-existe.yaml"},
		{"run", "-c", "../../exemplos/atual.yaml", "-replicas", "1"},
		{"compare"},
	}
	for _, args := range cases {
		if code, _, errs := runCLI(t, args...); code == 0 || errs == "" {
			t.Errorf("%v: deveria falhar com mensagem (código %d)", args, code)
		}
	}
}
