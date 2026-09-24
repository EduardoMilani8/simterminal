# simterminal

Simulador de eventos discretos (DES) para terminais de carga — portos, armazéns de grãos, centros de distribuição.

Responde perguntas do tipo "se eu abrir mais uma balança, a fila cai quanto?" sem precisar mexer na operação real.

## Por que isso existe

Num terminal, o gargalo raramente é o que parece. Fila de três horas no pátio sugere falta de doca, mas muitas vezes o problema é a balança única de quatro minutos, ou o pico de chegadas das 8h. Testar hipótese na vida real custa uma safra; aqui custa alguns segundos de CPU.

## Como funciona

O relógio não avança de minuto em minuto — ele salta direto para o instante do próximo evento agendado. O laço principal é:

1. tira o evento mais próximo de uma min-heap
2. adianta o relógio até ele
3. executa o evento, que normalmente agenda novos eventos à frente

Quando um caminhão chega e a balança está ocupada, ele entra na fila e nada é agendado — quem o acorda é o caminhão da frente ao terminar a pesagem.

## Estrutura

```
cmd/simterminal/     CLI — hoje roda a demo de uma balança
internal/des/        motor: relógio virtual, fila de eventos, distribuições
internal/stats/      acumulador de métricas (média, p95, máximo)
```

## Rodando

```bash
go test ./...                  # inclui a validação contra a fórmula M/M/1
go run ./cmd/simterminal
go run ./cmd/simterminal -intervalo 4 -pesagem 5 -trace
```

Flags: `-seed`, `-horizonte`, `-intervalo`, `-pesagem`, `-trace`.

## Validação

`TestMM1MatchesAnalyticFormula` simula uma fila M/M/1 com 300 mil clientes e compara a espera média com a fórmula fechada `Wq = ρ / (μ − λ)`. Se o motor tiver erro de lógica, esse teste quebra. Todo modelo novo deve ganhar uma âncora analítica parecida antes de virar base para decisão.

## Roadmap

- [x] **Dia 1** — motor DES puro, validado contra M/M/1
- [ ] **Dia 2** — abstração de recurso (capacidade N, fila FIFO, estatística de ocupação)
- [ ] **Dia 3** — modelo do terminal: chegadas com perfil por hora, tipos de carga, balança + docas
- [ ] **Dia 4** — múltiplas réplicas com sementes diferentes, média e intervalo de confiança
- [ ] **Dia 5** — cenários declarados em arquivo de configuração e saída comparativa

## Licença

MIT
