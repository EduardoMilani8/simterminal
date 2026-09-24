package scenario

import (
	"runtime"
	"sync"

	"github.com/eduardomilani8/simterminal/internal/terminal"
)

// Seeds devolve as sementes das n réplicas a partir de base. Cenários
// comparados devem usar as mesmas sementes (common random numbers): a
// diferença observada passa a vir da mudança estrutural, não do sorteio.
func Seeds(base int64, n int) []int64 {
	s := make([]int64, n)
	for i := range s {
		s[i] = base + int64(i)
	}
	return s
}

// RunReplicas roda uma réplica por semente, em paralelo. Cada réplica
// tem sua própria Engine e nada é compartilhado, então o resultado não
// depende da ordem em que as goroutines terminam.
func RunReplicas(cfg terminal.Config, seeds []int64) []terminal.Result {
	results := make([]terminal.Result, len(seeds))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, seed := range seeds {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, seed int64) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = terminal.Run(cfg, seed)
		}(i, seed)
	}
	wg.Wait()
	return results
}
