package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/eduardomilani8/simterminal/internal/antaq"
	"github.com/eduardomilani8/simterminal/internal/porto"
	"github.com/eduardomilani8/simterminal/internal/report"
)

const usagePorto = `simterminal porto — fila de navios a partir das atracações reais da ANTAQ

uso:
  simterminal porto baixar [-dir dados/antaq] [-anos 2022,2023,2024,2025] [-carga]
  simterminal porto bercos (-c PORTO.yaml | -porto NOME [-dados DIR] [-ano ANO])
  simterminal porto diagnostico -c PORTO.yaml
  simterminal porto cenarios    -c PORTO.yaml [-grupo NOME]
  simterminal porto capacidade  -c PORTO.yaml [-meta 5d] [-grupo NOME]
  simterminal porto previsao    -c PORTO.yaml [-grupo NOME]
  simterminal porto chegada     -c PORTO.yaml [-grupo NOME]

baixar       copia as tabelas de atracação do espelho dos dados da ANTAQ
bercos       lista os berços do porto com a carga de cada um: ponto de
             partida para montar os grupos do arquivo
diagnostico  o que aconteceu: espera, ocupação e se a espera é mesmo por
             falta de berço
cenarios     e se: berço a mais, fila única, operação mais rápida
capacidade   quantos navios por mês cabem sem a espera passar da meta
             (a agenda de janelas de atracação)
previsao     quanto um navio vai esperar, dito na hora em que chega;
             aprende no treino e mede o erro no período
chegada      chegada just-in-time: quanto fundeio se evita instruindo o
             navio a chegar na hora prevista, e quanto isso atrasa a fila

Exemplo em portos/paranagua.yaml.
`

func cmdPorto(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usagePorto)
		return 2
	}
	cmds := map[string]func([]string, io.Writer, io.Writer) int{
		"baixar":      portoBaixar,
		"bercos":      portoBercos,
		"diagnostico": portoDiagnostico,
		"cenarios":    portoCenarios,
		"capacidade":  portoCapacidade,
		"previsao":    portoPrevisao,
		"chegada":     portoChegada,
	}
	if f, ok := cmds[args[0]]; ok {
		return f(args[1:], stdout, stderr)
	}
	if h := args[0]; h == "-h" || h == "-help" || h == "--help" || h == "help" {
		fmt.Fprint(stdout, usagePorto)
		return 0
	}
	fmt.Fprintf(stderr, "comando desconhecido %q\n\n%s", args[0], usagePorto)
	return 2
}

func portoBaixar(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto baixar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "dados/antaq", "pasta de destino")
	anosFlag := fs.String("anos", "2022,2023,2024,2025", "anos a baixar, separados por vírgula")
	carga := fs.Bool("carga", false, "baixa também a tabela de carga (≈400 MB por ano, para saber a mercadoria de cada navio)")
	url := fs.String("url", antaq.Espelho, "zip com as tabelas da ANTAQ")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	anos, err := parseAnos(*anosFlag)
	if err != nil {
		return fail(stderr, err)
	}
	nomes := []string{"Mercadoria.txt"}
	for _, a := range anos {
		nomes = append(nomes, fmt.Sprintf("%dAtracacao.txt", a), fmt.Sprintf("%dTemposAtracacao.txt", a))
		if *carga {
			nomes = append(nomes, fmt.Sprintf("%dCarga.txt", a))
		}
	}
	fmt.Fprintf(stdout, "baixando para %s\n", *dir)
	if err := antaq.Baixar(*url, *dir, nomes, stdout); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintln(stdout, "pronto. Fonte: ANTAQ, Estatístico Aquaviário; espelho em doi:10.5281/zenodo.20549161 (CC-BY-4.0).")
	return 0
}

func parseAnos(s string) ([]int, error) {
	var anos []int
	for _, p := range strings.Split(s, ",") {
		a, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || a < 2010 || a > 2100 {
			return nil, fmt.Errorf("ano inválido %q", p)
		}
		anos = append(anos, a)
	}
	return anos, nil
}

// portoFlags são as opções dos comandos que leem um arquivo de porto.
type portoFlags struct {
	cfg   string
	grupo string
}

func (p *portoFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&p.cfg, "c", "", "arquivo do porto (YAML)")
	fs.StringVar(&p.grupo, "grupo", "", "só este grupo de berços")
}

// carregar lê o arquivo e monta os grupos (todos ou só o pedido).
func (p *portoFlags) carregar(fs *flag.FlagSet, args []string, stderr io.Writer, carga bool) (*porto.Config, []*porto.DadosGrupo, []antaq.Atracacao, int) {
	rest, err := parseInterleaved(fs, args)
	if err != nil {
		return nil, nil, nil, 2
	}
	if p.cfg == "" && len(rest) == 1 {
		p.cfg = rest[0]
	} else if len(rest) > 0 {
		fmt.Fprintf(stderr, "argumentos a mais: %v\n", rest)
		return nil, nil, nil, 2
	}
	if p.cfg == "" {
		fmt.Fprintf(stderr, "informe o porto: simterminal porto %s -c portos/paranagua.yaml\n", fs.Name()[len("porto "):])
		return nil, nil, nil, 2
	}
	cfg, err := porto.Carregar(p.cfg)
	if err != nil {
		return nil, nil, nil, fail(stderr, err)
	}
	as, err := porto.CarregarAtracacoes(&cfg, carga)
	if err != nil {
		return nil, nil, nil, fail(stderr, dicaBaixar(err))
	}
	gs, err := porto.MontarGrupos(&cfg, as)
	if err != nil {
		return nil, nil, nil, fail(stderr, err)
	}
	if p.grupo != "" {
		var sel []*porto.DadosGrupo
		for _, g := range gs {
			if g.Grupo.Nome == p.grupo {
				sel = append(sel, g)
			}
		}
		if len(sel) == 0 {
			var nomes []string
			for _, g := range gs {
				nomes = append(nomes, g.Grupo.Nome)
			}
			return nil, nil, nil, fail(stderr, fmt.Errorf("grupo %q não existe; os grupos são: %s", p.grupo, strings.Join(nomes, "; ")))
		}
		gs = sel
	}
	return &cfg, gs, as, 0
}

// dicaBaixar troca "arquivo não existe" por como resolver.
func dicaBaixar(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) && os.IsNotExist(pe.Err) {
		return fmt.Errorf("não encontrei %s\n  os dados da ANTAQ não foram baixados: rode simterminal porto baixar", pe.Path)
	}
	return err
}

func nomesMercadoria(dir string) map[string]string {
	m, err := antaq.Mercadorias(filepath.Join(dir, "Mercadoria.txt"))
	if err != nil {
		return nil
	}
	return m
}

func portoDiagnostico(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto diagnostico", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf portoFlags
	pf.register(fs)
	cfg, gs, _, code := pf.carregar(fs, args, stderr, true)
	if cfg == nil {
		return code
	}
	var ds []porto.Diagnostico
	for _, g := range gs {
		ds = append(ds, porto.Diagnosticar(g))
	}
	report.WritePortoDiagnostico(stdout, cfg, ds, nomesMercadoria(cfg.PastaDados()))
	return 0
}

func portoCenarios(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto cenarios", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf portoFlags
	pf.register(fs)
	cfg, gs, _, code := pf.carregar(fs, args, stderr, false)
	if cfg == nil {
		return code
	}
	fmt.Fprintf(stdout, "%s — replay de %s com os navios, a ordem do line-up e os tempos reais; só muda o que o\n", cfg.Porto, periodoAno(cfg))
	fmt.Fprintln(stdout, "cenário muda. Faixas: da leitura conservadora (navio pronto só depois de quem o ultrapassou) à")
	fmt.Fprintln(stdout, "otimista (pronto ao chegar). A variação é contra o modelo sem mudança, na mesma leitura.")
	for _, g := range gs {
		fmt.Fprintln(stdout)
		report.WritePortoCenarios(stdout, cfg, g.Grupo.Nome, porto.ResumoReal(g), porto.Comparar(g, cfg.Cenarios))
	}
	return 0
}

func periodoAno(cfg *porto.Config) string { return janelaLabel(cfg.Periodo) }

func portoCapacidade(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto capacidade", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf portoFlags
	pf.register(fs)
	metaFlag := fs.String("meta", "5d", "espera mediana máxima aceitável: 72h, 5d...")
	cfg, gs, _, code := pf.carregar(fs, args, stderr, false)
	if cfg == nil {
		return code
	}
	meta, err := parseHoras(*metaFlag)
	if err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "%s — quantos navios por mês cabem com espera mediana de até %s.\n", cfg.Porto, report.Dias(meta))
	fmt.Fprintln(stdout, "Os mesmos navios de "+periodoAno(cfg)+", chegando mais juntos ou mais espaçados.")
	for _, g := range gs {
		cenarios := []porto.Cenario{porto.SemMudanca()}
		cenarios[0].Nome = "como opera hoje"
		for _, sc := range cfg.Cenarios {
			if (sc.Grupo == "" || sc.Grupo == g.Grupo.Nome) && sc.Demanda == nil {
				cenarios = append(cenarios, sc)
			}
		}
		var cs []porto.Capacidade
		for _, sc := range cenarios {
			cs = append(cs, porto.CalcularCapacidade(g, sc, meta))
		}
		fmt.Fprintln(stdout)
		report.WritePortoCapacidade(stdout, g.Grupo.Nome, cs)
	}
	return 0
}

func parseHoras(s string) (float64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "d"):
		mult, s = 24, strings.TrimSuffix(s, "d")
	case strings.HasSuffix(s, "h"):
		s = strings.TrimSuffix(s, "h")
	}
	x, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil || x <= 0 {
		return 0, fmt.Errorf("meta inválida: use algo como 72h ou 5d")
	}
	return x * mult, nil
}

func portoPrevisao(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto previsao", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf portoFlags
	pf.register(fs)
	cfg, gs, _, code := pf.carregar(fs, args, stderr, false)
	if cfg == nil {
		return code
	}
	if cfg.Treino.Inicio.IsZero() {
		return fail(stderr, fmt.Errorf("%s: defina treino (antes do período) para a regressão aprender", pf.cfg))
	}
	fmt.Fprintf(stdout, "%s — previsão da espera na chegada, medida nos navios de %s.\n", cfg.Porto, periodoAno(cfg))
	fmt.Fprintln(stdout, "Nenhum método vê o que aconteceu depois da chegada do navio. O método de cada grupo é escolhido")
	fmt.Fprintf(stdout, "pelo erro em %s (treino), não pelo erro abaixo — escolher olhando o resultado seria colar a resposta.\n",
		janelaLabel(cfg.Treino))
	for _, g := range gs {
		fmt.Fprintln(stdout)
		p := porto.NovoPrevisor(g)
		escolhido := porto.Melhor(p.Avaliar(cfg.Treino))
		report.WritePortoPrevisao(stdout, g.Grupo.Nome, porto.ResumoReal(g), p.Avaliar(cfg.Periodo), escolhido)
	}
	return 0
}

func janelaLabel(j porto.Janela) string {
	return j.Inicio.Format("02/01/2006") + " a " + j.Fim.Add(-time.Second).Format("02/01/2006")
}

func portoChegada(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto chegada", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var pf portoFlags
	pf.register(fs)
	cfg, gs, _, code := pf.carregar(fs, args, stderr, false)
	if cfg == nil {
		return code
	}
	if cfg.Treino.Inicio.IsZero() {
		return fail(stderr, fmt.Errorf("%s: defina treino para a previsão que orienta a chegada", pf.cfg))
	}
	fmt.Fprintf(stdout, "%s — chegada just-in-time, navios de %s. Em vez de correr para o fundeadouro, o navio\n", cfg.Porto, periodoAno(cfg))
	fmt.Fprintln(stdout, "vem na hora marcada (pela previsão de espera) ou espera no mar até ser chamado (fila virtual). Cada")
	fmt.Fprintln(stdout, "navio vai para o seu berço real; se o próximo da vez não chegou, o berço atende quem estiver pronto.")
	fmt.Fprintln(stdout, "A referência é o mesmo replay com os navios chegando como chegaram.")
	for _, g := range gs {
		fmt.Fprintln(stdout)
		p := porto.NovoPrevisor(g)
		m := porto.Melhor(p.Avaliar(cfg.Treino))
		report.WritePortoChegada(stdout, g.Grupo.Nome+" — previsão: "+m.String(), porto.AvaliarChegada(g, p, m, porto.Politicas()))
	}
	return 0
}

// portoBercos lista os berços de um porto com o que cada um movimentou.
func portoBercos(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("porto bercos", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("c", "", "arquivo do porto (YAML)")
	nome := fs.String("porto", "", "nome do porto como a ANTAQ escreve, se não houver arquivo")
	dir := fs.String("dados", "dados/antaq", "pasta dos dados, se não houver arquivo")
	ano := fs.Int("ano", 0, "ano, se não houver arquivo (padrão: o último completo baixado)")
	if _, err := parseInterleaved(fs, args); err != nil {
		return 2
	}

	var cfg porto.Config
	grupoDe := map[string]string{}
	switch {
	case *cfgPath != "":
		c, err := porto.Carregar(*cfgPath)
		if err != nil {
			return fail(stderr, err)
		}
		cfg = c
		for _, g := range cfg.Grupos {
			for _, b := range g.Bercos {
				grupoDe[b] = g.Nome
			}
		}
	case *nome != "":
		a := *ano
		if a == 0 {
			a = time.Now().Year() - 1
			for ; a > 2010; a-- {
				if _, err := os.Stat(filepath.Join(*dir, fmt.Sprintf("%dAtracacao.txt", a+1))); err == nil {
					break // o ano seguinte existe: este está completo
				}
			}
		}
		cfg = porto.Config{Porto: *nome, Dados: *dir, Anos: []int{a}}
		ini := time.Date(a, 1, 1, 0, 0, 0, 0, time.UTC)
		cfg.Periodo = porto.Janela{Inicio: porto.Data{Time: ini}, Fim: porto.Data{Time: ini.AddDate(1, 0, 0)}}
	default:
		fmt.Fprintln(stderr, "informe -c PORTO.yaml ou -porto NOME")
		return 2
	}

	as, err := porto.CarregarAtracacoes(&cfg, true)
	if err != nil {
		return fail(stderr, dicaBaixar(err))
	}
	nomes := nomesMercadoria(cfg.PastaDados())

	type info struct {
		id, nome, terminal string
		navios             int
		esperas            []float64
		cargas             map[string]int
	}
	bs := map[string]*info{}
	for _, a := range as {
		if !cfg.Periodo.Contem(a.Chegada) || !a.Completa() {
			continue
		}
		b := bs[a.Berco]
		if b == nil {
			b = &info{id: a.Berco, nome: a.NomeBerco, terminal: a.Terminal, cargas: map[string]int{}}
			bs[a.Berco] = b
		}
		b.navios++
		b.esperas = append(b.esperas, a.Espera())
		if a.Mercadoria != "" {
			// por nome: três NCM de adubo contam como "Adubos"
			b.cargas[nomeOu(nomes, a.Mercadoria)]++
		}
	}
	var lista []*info
	for _, b := range bs {
		lista = append(lista, b)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].id < lista[j].id })

	fmt.Fprintf(stdout, "%s — berços com navios chegando em %s\n\n", cfg.Porto, periodoAno(&cfg))
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "  berço\tnome\tterminal\tnavios\tespera mediana\tcargas principais\tgrupo\n")
	for _, b := range lista {
		sort.Float64s(b.esperas)
		type kv struct {
			k string
			n int
		}
		var cs []kv
		for k, n := range b.cargas {
			cs = append(cs, kv{k, n})
		}
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].n == cs[j].n {
				return cs[i].k < cs[j].k
			}
			return cs[i].n > cs[j].n
		})
		var ps []string
		for _, c := range cs[:min(3, len(cs))] {
			ps = append(ps, fmt.Sprintf("%s %d", encurtar(c.k, 28), c.n))
		}
		g := grupoDe[b.id]
		if g == "" {
			g = "—"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%s\t%s\t%s\n", b.id, b.nome, encurtar(b.terminal, 36), b.navios,
			report.Dias(b.esperas[len(b.esperas)/2]), strings.Join(ps, ", "), g)
	}
	tw.Flush()
	if len(lista) > 0 && len(lista[0].cargas) == 0 {
		fmt.Fprintln(stdout, "\n  sem a tabela de carga: rode simterminal porto baixar -carga para ver a mercadoria de cada berço.")
	}
	return 0
}

func nomeOu(nomes map[string]string, cod string) string {
	if n, ok := nomes[cod]; ok && n != "" {
		return n
	}
	return cod
}

func encurtar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
