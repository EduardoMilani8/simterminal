package antaq

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// lerTabela percorre um arquivo da ANTAQ linha a linha e chama fn com as
// colunas pedidas, na ordem pedida. As colunas são achadas pelo nome no
// cabeçalho, não pela posição: a ANTAQ já mudou a ordem entre anos.
func lerTabela(path string, colunas []string, fn func([]string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return lerTabelaDe(f, path, colunas, fn)
}

func lerTabelaDe(r io.Reader, nome string, colunas []string, fn func([]string) error) error {
	cr := csv.NewReader(bufio.NewReaderSize(r, 1<<20))
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = true

	cab, err := cr.Read()
	if err != nil {
		return fmt.Errorf("%s: cabeçalho: %w", nome, err)
	}
	pos := map[string]int{}
	for i, c := range cab {
		pos[strings.TrimSpace(strings.TrimPrefix(c, "\ufeff"))] = i
	}
	idx := make([]int, len(colunas))
	for i, c := range colunas {
		p, ok := pos[c]
		if !ok {
			return fmt.Errorf("%s: coluna %q não encontrada", nome, c)
		}
		idx[i] = p
	}

	v := make([]string, len(colunas))
	for linha := 2; ; linha++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: linha %d: %w", nome, linha, err)
		}
		for i, p := range idx {
			if p < len(rec) {
				v[i] = strings.TrimSpace(rec[p])
			} else {
				v[i] = ""
			}
		}
		if err := fn(v); err != nil {
			return fmt.Errorf("%s: linha %d: %w", nome, linha, err)
		}
	}
}

// data lê "dd/mm/aaaa hh:mm:ss". Vazio vira o instante zero. O horário é
// o local do porto; guardamos como UTC só para não haver fuso no meio
// das contas.
func data(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("02/01/2006 15:04:05", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("data inválida %q", s)
	}
	return t, nil
}

// numero lê um número com vírgula decimal. Vazio ou "Valor Discrepante"
// (a marca da ANTAQ para tempos que ela mesma considera suspeitos) viram
// NaN.
func numero(s string) float64 {
	if s == "" {
		return math.NaN()
	}
	x, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil {
		return math.NaN()
	}
	return x
}

// Mercadorias lê a tabela de mercadorias da ANTAQ (Mercadoria.txt) e
// devolve o nome curto de cada código.
func Mercadorias(path string) (map[string]string, error) {
	m := map[string]string{}
	err := lerTabela(path, []string{"CDMercadoria", "Nomenclatura Simplificada Mercadoria", "Mercadoria"},
		func(v []string) error {
			nome := v[1]
			if nome == "" {
				nome = v[2]
			}
			m[v[0]] = nome
			return nil
		})
	return m, err
}
