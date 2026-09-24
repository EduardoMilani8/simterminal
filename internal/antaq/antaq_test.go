package antaq

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Linhas no formato exato dos arquivos da ANTAQ: BOM, CRLF, ponto e
// vírgula, aspas em campo com ";" dentro e colunas que o leitor ignora.
const atracacao2024 = "\ufeffIDAtracacao;CDTUP;IDBerco;Berço;Porto Atracação;Coordenadas;Apelido Instalação Portuária;Complexo Portuário;Tipo da Autoridade Portuária;Data Atracação;Data Chegada;Data Desatracação;Data Início Operação;Data Término Operação;Ano;Mes;Tipo de Operação;Tipo de Navegação da Atracação;Nacionalidade do Armador;FlagMCOperacaoAtracacao;Terminal;Município;UF;SGUF;Região Geográfica;Região Hidrográfica;Instalação Portuária em Rio;Nº da Capitania;Nº do IMO\r\n" +
	"1;BRPNG;PNG0212;Berço 212;Paranaguá;-48.5,-25.5;;Paranaguá;Porto Organizado;10/03/2024 08:00:00;01/03/2024 06:00:00;13/03/2024 20:00:00;10/03/2024 10:00:00;13/03/2024 18:00:00;2024;mar;Movimentação da Carga;Longo Curso;2;1;\"Corredor; Leste\";Paranaguá;Paraná;PR;Sul;;Não;;9123456\r\n" +
	"2;BRPNG;PNG0212;Berço 212;Paranaguá;-48.5,-25.5;;Paranaguá;Porto Organizado;14/03/2024 00:00:00;12/03/2024 00:00:00;16/03/2024 00:00:00;;;2024;mar;Movimentação da Carga;Longo Curso;2;1;Corredor Leste;Paranaguá;Paraná;PR;Sul;;Não;;9999999\r\n" +
	"3;BRSSZ;SSZ001;Berço 1;Santos;0,0;;Santos;Porto Organizado;14/03/2024 00:00:00;12/03/2024 00:00:00;16/03/2024 00:00:00;;;2024;mar;Movimentação da Carga;Longo Curso;2;1;X;Santos;SP;SP;Sudeste;;Não;;1\r\n" +
	"4;BRPNG;PNG0212;Berço 212;Paranaguá;0,0;;Paranaguá;Porto Organizado;14/03/2024 00:00:00;12/03/2024 00:00:00;16/03/2024 00:00:00;;;2024;mar;Apoio;Apoio Portuário;1;0;X;Paranaguá;PR;PR;Sul;;Não;;\r\n"

// Colunas em outra ordem: o leitor acha pelo nome.
const tempos2024 = "\ufeffIDAtracacao;TEsperaInicioOp;TEsperaAtracacao;TOperacao;TEsperaDesatracacao;TAtracado;TEstadia\r\n" +
	"1;2;218,5;80;2;84;302\r\n" +
	"2;Valor Discrepante;48;;;48;96\r\n"

const carga2024 = "\ufeffIDCarga;IDAtracacao;CDMercadoria;Natureza da Carga;Sentido;VLPesoCargaBruta;FlagMCOperacaoCarga\r\n" +
	"10;1;2304;Granel Sólido;Embarcados;20000,5;1\r\n" +
	"11;1;1201;Granel Sólido;Embarcados;45000;1\r\n" +
	"12;1;CA01;Carga Geral;Embarcados;99999;0\r\n"

// A mesma atracação 2 volta no arquivo de 2025: deve contar uma vez só.
const atracacao2025 = "\ufeffIDAtracacao;CDTUP;IDBerco;Berço;Porto Atracação;Terminal;Data Chegada;Data Atracação;Data Início Operação;Data Término Operação;Data Desatracação;Tipo de Operação;Tipo de Navegação da Atracação;Nº do IMO\r\n" +
	"2;BRPNG;PNG0212;Berço 212;Paranaguá;Corredor Leste;12/03/2024 00:00:00;14/03/2024 00:00:00;;;16/03/2024 00:00:00;Movimentação da Carga;Longo Curso;9999999\r\n" +
	"5;BRPNG;PNG0213;Berço 213;Paranaguá;Corredor Leste;01/01/2025 00:00:00;05/01/2025 00:00:00;;;07/01/2025 00:00:00;Movimentação da Carga;Longo Curso;1\r\n"

const tempos2025 = "\ufeffIDAtracacao;TEsperaAtracacao;TEsperaInicioOp;TOperacao;TEsperaDesatracacao;TAtracado;TEstadia\r\n"

func escrever(t *testing.T, dir string, arquivos map[string]string) {
	t.Helper()
	for n, c := range arquivos {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCarregar(t *testing.T) {
	dir := t.TempDir()
	escrever(t, dir, map[string]string{
		"2024Atracacao.txt": atracacao2024, "2024TemposAtracacao.txt": tempos2024, "2024Carga.txt": carga2024,
		"2025Atracacao.txt": atracacao2025, "2025TemposAtracacao.txt": tempos2025,
	})
	as, err := Carregar(dir, []int{2024, 2025}, DoPorto("Paranaguá"), true)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	// 3 é de Santos; 2 aparece nos dois anos; 4 é apoio, mas ocupa berço
	if got := strings.Join(ids, ","); got != "1,2,4,5" {
		t.Fatalf("atracações = %s, esperava 1,2,4,5", got)
	}

	a := as[0]
	if a.Terminal != "Corredor; Leste" {
		t.Errorf("campo entre aspas: %q", a.Terminal)
	}
	if want := time.Date(2024, 3, 1, 6, 0, 0, 0, time.UTC); !a.Chegada.Equal(want) {
		t.Errorf("chegada = %v, esperava %v", a.Chegada, want)
	}
	if a.Espera() != 218 || a.NoBerco() != 84 {
		t.Errorf("espera %.1f h, no berço %.1f h; esperava 218 e 84", a.Espera(), a.NoBerco())
	}
	if a.TEspera != 218.5 || a.TEsperaInicioOp != 2 {
		t.Errorf("tempos da ANTAQ: TEspera %v, T2 %v", a.TEspera, a.TEsperaInicioOp)
	}
	// carga principal é a de maior peso entre as que contam como movimentação
	if a.Mercadoria != "1201" || a.Toneladas != 65000.5 {
		t.Errorf("carga = %s, %.1f t; esperava 1201, 65000.5 t", a.Mercadoria, a.Toneladas)
	}

	b := as[1]
	if !math.IsNaN(b.TEsperaInicioOp) || !math.IsNaN(b.TOperacao) {
		t.Errorf("valor discrepante ou vazio deveria virar NaN: %v %v", b.TEsperaInicioOp, b.TOperacao)
	}
	if !b.InicioOp.IsZero() || !b.Completa() {
		t.Errorf("data vazia deveria ser zero e a atracação continuar completa")
	}
}

func TestColunaFaltando(t *testing.T) {
	dir := t.TempDir()
	escrever(t, dir, map[string]string{
		"2024Atracacao.txt":       "\ufeffIDAtracacao;CDTUP\r\n1;BRPNG\r\n",
		"2024TemposAtracacao.txt": tempos2024,
	})
	_, err := Carregar(dir, []int{2024}, nil, false)
	if err == nil || !strings.Contains(err.Error(), "não encontrada") {
		t.Fatalf("esperava erro de coluna ausente, veio %v", err)
	}
}
