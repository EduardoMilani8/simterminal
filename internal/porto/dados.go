package porto

import (
	"fmt"

	"github.com/eduardomilani8/simterminal/internal/antaq"
)

// CarregarAtracacoes lê do disco as atracações do porto do arquivo, de
// todos os berços. Com carga, traz a mercadoria de cada uma (lento: a
// tabela de carga tem 400 MB por ano).
func CarregarAtracacoes(c *Config, carga bool) ([]antaq.Atracacao, error) {
	as, err := antaq.Carregar(c.PastaDados(), c.Anos, antaq.DoPorto(c.Porto), carga)
	if err != nil {
		return nil, err
	}
	if len(as) == 0 {
		return nil, fmt.Errorf("nenhuma atracação de %q nos anos %v: confira a grafia do porto como a ANTAQ escreve", c.Porto, c.Anos)
	}
	return as, nil
}

// CarregarGrupos monta os grupos de berços do arquivo.
func CarregarGrupos(c *Config) ([]*DadosGrupo, error) {
	as, err := CarregarAtracacoes(c, false)
	if err != nil {
		return nil, err
	}
	return MontarGrupos(c, as)
}

// MontarGrupos monta os grupos a partir de atracações já lidas.
func MontarGrupos(c *Config, as []antaq.Atracacao) ([]*DadosGrupo, error) {
	var gs []*DadosGrupo
	for _, g := range c.Grupos {
		d := MontarGrupo(g, as, c.Periodo)
		usados := map[string]bool{}
		for _, n := range d.Navios {
			usados[n.Berco] = true
		}
		for _, b := range g.Bercos {
			if !usados[b] {
				return nil, fmt.Errorf("grupo %q: berço %s sem nenhuma atracação nos anos %v", g.Nome, b, c.Anos)
			}
		}
		gs = append(gs, d)
	}
	return gs, nil
}
