package antaq

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
)

// Espelho é o arquivo do Zenodo com as tabelas brutas do Estatístico
// Aquaviário de 2018 a jun/2025, publicado sob CC-BY-4.0 junto com
// Pacheco et al., "Methodological Pitfalls in Predicting Ship Turnaround
// Time at Brazilian Ports", IEEE T-ITS, 2026 (doi:10.5281/zenodo.20549161).
//
// O site da própria ANTAQ fica atrás de um desafio do Cloudflare que
// bloqueia download automático; os arquivos do espelho são os mesmos, e
// quem baixar à mão do site pode apontar -dir para eles.
const Espelho = "https://zenodo.org/api/records/20549767/files/antaq-turnaround-data-v1.2.zip/content"

// Baixar copia de url (um zip) para dir apenas os arquivos cujo nome base
// está em nomes. Lê só o índice do zip e os trechos pedidos, por HTTP
// Range: o zip do espelho tem 5,5 GB e as tabelas de atracação somam
// poucas dezenas de MB. Arquivos que já existem em dir são pulados.
func Baixar(url, dir string, nomes []string, log io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var faltam []string
	for _, n := range nomes {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			fmt.Fprintf(log, "  já existe  %s\n", n)
			continue
		}
		faltam = append(faltam, n)
	}
	if len(faltam) == 0 {
		return nil
	}

	ra, err := abrirRemoto(url)
	if err != nil {
		return err
	}
	z, err := zip.NewReader(ra, ra.tamanho)
	if err != nil {
		return fmt.Errorf("lendo o índice do zip: %w", err)
	}
	porNome := map[string]*zip.File{}
	for _, f := range z.File {
		porNome[path.Base(f.Name)] = f
	}
	for _, n := range faltam {
		f, ok := porNome[n]
		if !ok {
			return fmt.Errorf("%s não está no arquivo", n)
		}
		fmt.Fprintf(log, "  baixando   %s (%.1f MB)\n", n, float64(f.CompressedSize64)/1e6)
		if err := extrair(f, filepath.Join(dir, n)); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
	}
	return nil
}

func extrair(f *zip.File, dst string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := dst + ".parcial"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// remoto é um io.ReaderAt sobre HTTP Range, com cache de blocos: o leitor
// de zip faz muitas leituras pequenas e cada uma viraria uma requisição.
type remoto struct {
	url     string
	tamanho int64

	mu     sync.Mutex
	blocos map[int64][]byte
	ordem  []int64
}

const tamBloco = 4 << 20
const maxBlocos = 16

func abrirRemoto(url string) (*remoto, error) {
	resp, err := http.Head(url)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	n, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("%s: tamanho desconhecido", url)
	}
	// segue o redirecionamento uma vez só
	return &remoto{url: resp.Request.URL.String(), tamanho: n, blocos: map[int64][]byte{}}, nil
}

func (r *remoto) ReadAt(p []byte, off int64) (int, error) {
	lidos := 0
	for lidos < len(p) {
		pos := off + int64(lidos)
		if pos >= r.tamanho {
			return lidos, io.EOF
		}
		b, err := r.bloco(pos / tamBloco)
		if err != nil {
			return lidos, err
		}
		lidos += copy(p[lidos:], b[pos%tamBloco:])
	}
	return lidos, nil
}

func (r *remoto) bloco(i int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.blocos[i]; ok {
		return b, nil
	}
	ini := i * tamBloco
	fim := min(ini+tamBloco, r.tamanho) - 1
	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", ini, fim))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("o servidor não aceitou leitura parcial (HTTP %d)", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != fim-ini+1 {
		return nil, fmt.Errorf("bloco %d veio com %d bytes, esperava %d", i, len(b), fim-ini+1)
	}
	if len(r.ordem) >= maxBlocos {
		delete(r.blocos, r.ordem[0])
		r.ordem = r.ordem[1:]
	}
	r.blocos[i] = b
	r.ordem = append(r.ordem, i)
	return b, nil
}
