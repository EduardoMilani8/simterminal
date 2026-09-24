// Comando simterminal: simula um terminal de carga a partir de cenários
// em YAML e compara as alternativas.
//
//	simterminal run -c exemplos/atual.yaml
//	simterminal compare exemplos/atual.yaml exemplos/maisbalanca.yaml exemplos/maisdoca.yaml
//	simterminal demo -trace
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/eduardomilani8/simterminal/internal/report"
	"github.com/eduardomilani8/simterminal/internal/scenario"
)

const usage = `simterminal — simulador de eventos discretos para terminais de carga

uso:
  simterminal run -c CENARIO.yaml [-replicas N] [-semente S] [-csv ARQ] [-caminhoes ARQ]
  simterminal compare CENARIO.yaml... [-replicas N] [-semente S] [-csv ARQ]
  simterminal demo [-trace] [-intervalo MIN] [-pesagem MIN] [-horizonte MIN] [-seed S]

run       roda um cenário e mostra tempo no terminal, filas e o gargalo
compare   roda vários cenários com as mesmas sementes e monta a tabela de
          decisão; o primeiro é a referência das diferenças
demo      a demo do motor: uma balança, um turno

Exemplos de cenário em exemplos/.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:], stdout, stderr)
	case "compare":
		return cmdCompare(args[1:], stdout, stderr)
	case "demo":
		return cmdDemo(args[1:], stdout, stderr)
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "comando desconhecido %q\n\n%s", args[0], usage)
	return 2
}

// runFlags são as opções comuns a run e compare. Zero em replicas ou
// semente significa "usar o que está no arquivo".
type runFlags struct {
	replicas int
	seed     int64
	csvPath  string
}

func (f *runFlags) register(fs *flag.FlagSet) {
	fs.IntVar(&f.replicas, "replicas", 0, "número de réplicas (padrão: o do arquivo)")
	fs.Int64Var(&f.seed, "semente", 0, "semente da primeira réplica (padrão: a do arquivo)")
	fs.StringVar(&f.csvPath, "csv", "", "grava o resumo em CSV neste arquivo")
}

// seeds decide as sementes: as das flags, se informadas, senão as do
// cenário de referência.
func (f *runFlags) seeds(ref scenario.Scenario) ([]int64, error) {
	n, s := ref.Replicas, ref.Seed
	if f.replicas != 0 {
		n = f.replicas
	}
	if f.seed != 0 {
		s = f.seed
	}
	if n < 2 {
		return nil, fmt.Errorf("-replicas %d: precisa de pelo menos 2 para ter intervalo de confiança", n)
	}
	return scenario.Seeds(s, n), nil
}

func cmdRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var rf runFlags
	rf.register(fs)
	path := fs.String("c", "", "arquivo do cenário (YAML)")
	trucksPath := fs.String("caminhoes", "", "grava a jornada de cada caminhão da primeira réplica em CSV")
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return 2
	}
	if *path == "" && len(rest) == 1 {
		*path = rest[0]
	} else if len(rest) > 0 {
		fmt.Fprintf(stderr, "argumentos a mais: %v\n", rest)
		return 2
	}
	if *path == "" {
		fmt.Fprintln(stderr, "informe o cenário: simterminal run -c CENARIO.yaml")
		return 2
	}

	sc, err := scenario.Load(*path)
	if err != nil {
		return fail(stderr, err)
	}
	seeds, err := rf.seeds(sc)
	if err != nil {
		return fail(stderr, err)
	}
	s := scenario.Run(sc.Name, sc.Config, seeds)
	report.WriteRun(stdout, &s)

	if rf.csvPath != "" {
		if err := writeFile(rf.csvPath, func(w io.Writer) error {
			return report.WriteCSV(w, []*scenario.Summary{&s})
		}); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "\nresumo gravado em %s\n", rf.csvPath)
	}
	if *trucksPath != "" {
		if err := writeFile(*trucksPath, func(w io.Writer) error {
			return report.WriteTrucksCSV(w, &s.Results[0])
		}); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "jornada dos caminhões (semente %d) gravada em %s\n", seeds[0], *trucksPath)
	}
	return 0
}

func cmdCompare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var rf runFlags
	rf.register(fs)
	paths, err := parseInterleaved(fs, args)
	if err != nil {
		return 2
	}
	if len(paths) == 0 {
		fmt.Fprintln(stderr, "informe os cenários: simterminal compare atual.yaml alternativa.yaml ...")
		return 2
	}

	var scs []scenario.Scenario
	for _, p := range paths {
		sc, err := scenario.Load(p)
		if err != nil {
			return fail(stderr, err)
		}
		scs = append(scs, sc)
	}
	// common random numbers: todos rodam com as mesmas sementes
	seeds, err := rf.seeds(scs[0])
	if err != nil {
		return fail(stderr, err)
	}

	var ss []*scenario.Summary
	for _, sc := range scs {
		s := scenario.Run(sc.Name, sc.Config, seeds)
		ss = append(ss, &s)
	}
	report.WriteCompare(stdout, ss)

	if rf.csvPath != "" {
		if err := writeFile(rf.csvPath, func(w io.Writer) error { return report.WriteCSV(w, ss) }); err != nil {
			return fail(stderr, err)
		}
		fmt.Fprintf(stdout, "\nresumo gravado em %s\n", rf.csvPath)
	}
	return 0
}

// parseInterleaved aceita flags antes, entre ou depois dos argumentos
// posicionais: "compare a.yaml b.yaml -csv x.csv" funciona.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func writeFile(path string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fail(stderr io.Writer, err error) int {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		fmt.Fprintf(stderr, "erro: não consegui abrir %s: %v\n", pathErr.Path, pathErr.Err)
		return 1
	}
	fmt.Fprintf(stderr, "erro: %v\n", err)
	return 1
}
