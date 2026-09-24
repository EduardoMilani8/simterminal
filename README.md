# simterminal

Simulador de eventos discretos (DES) para terminais de carga — portos, armazéns de grãos, centros de distribuição.

Responde perguntas do tipo "se eu abrir mais uma balança, a fila cai quanto?" sem precisar mexer na operação real. O plano completo está em [mapa.md](mapa.md).

## Por que isso existe

Num terminal, o gargalo raramente é o que parece. Fila de três horas no pátio sugere falta de doca, mas muitas vezes o problema é a balança única de quatro minutos, ou o pico de chegadas das 8h. Testar hipótese na vida real custa uma safra; aqui custa alguns segundos de CPU.

## Rodando

```bash
go test ./...                                   # todas as âncoras de validação
go run ./cmd/simterminal run -c exemplos/atual.yaml
make compare                                    # compara todos os exemplos
```

### `run` — um cenário

```
$ simterminal run -c exemplos/atual.yaml
atual — 30 réplicas (sementes 1–30), warm-up 1h00, IC 95%

  tempo no terminal    3h04 ± 11min
  p95 do tempo         4h49 ± 16min
  fila máx. no pátio   47 ± 3 caminhões
  caminhões atendidos  141 ± 4
  último caminhão sai  13h34 ± 16min após a abertura (portão fecha em 12h00)

  recurso             cap.  carga  utilização  espera média  fila máx.
  balança de entrada     1    97%    96% ± 1%  1h56 ± 10min     47 ± 3
  docas                 12    75%    74% ± 1%          0min          2
  balança de saída       1    97%    91% ± 1%  15min ± 2min      8 ± 1

  gargalo: balança de entrada — espera média de 1h56 por caminhão, utilização de 96%.
```

Opções: `-replicas N`, `-semente S`, `-csv resumo.csv` e `-caminhoes jornada.csv` (os marcos de tempo de cada caminhão da primeira réplica — onde exatamente cada um perdeu o dia).

### `compare` — a tabela de decisão

```
$ simterminal compare exemplos/atual.yaml exemplos/maisbalanca.yaml exemplos/maisbalancas.yaml \
                      exemplos/maisdoca.yaml exemplos/agendamento.yaml exemplos/balancaunica.yaml
6 cenários com as mesmas 30 réplicas (sementes 1–30), IC 95%

Cenário                Recursos                        Tempo médio   p95   Fila máx  Util. balança  Gargalo
atual                  1+1 bal., 12 docas              3h04 ± 11min  4h49  47        96%            balança de entrada
+ 1 balança            2+1 bal., 12 docas              2h49 ± 10min  4h35  13        50%            balança de saída
+ 1 balança cada lado  2+2 bal., 12 docas              1h45 ± 8min   2h56  13        50%            docas
+ 1 doca               1+1 bal., 13 docas              3h04 ± 11min  4h49  47        96%            balança de entrada
50% agendado           1+1 bal., 12 docas, 50% agend.  2h09 ± 8min   3h15  26        97%            balança de entrada
balança única          1 bal. única, 12 docas          saturado      —     104       99%            balança

  ⚠ balança única: saturado — balança recebe 193% da própria capacidade. A fila cresce
    sem parar e as médias passam a depender do horizonte escolhido, não do sistema.

Tempo médio comparado a "atual" (pareado, mesmas sementes):
  + 1 balança            −15min ± 2min   mais rápido
  + 1 balança cada lado  −1h18 ± 5min    mais rápido
  + 1 doca               0min            sem diferença detectável
  50% agendado           −55min ± 12min  mais rápido
  balança única          +8h00 ± 18min   saturado — não compare pela média

Conclusão: "+ 1 balança cada lado" é o melhor — 1h18 ± 5min a menos por caminhão que "atual".
```

Repare no que a tabela mostra: a doca extra não muda nada, e uma balança a mais só na entrada ganha 15 minutos, porque o gargalo pula para a balança de saída, que está tão carregada quanto. Só abrindo dos dois lados a fila cai de verdade — e aí o próximo gargalo passa a ser as docas.

Todos os cenários rodam com as mesmas sementes (*common random numbers*): o mesmo caminhão chega na mesma hora e demora o mesmo tanto em todos eles. Por isso a diferença pareada tem intervalo bem mais estreito que o de cada cenário isolado.

`-csv` grava o resumo em formato longo (`cenario, recurso, metrica, media, ic95, replicas`), pronto para tabela dinâmica.

### `demo`

A demo do dia 1: uma balança só, um turno. Com `-trace` dá para ver o relógio saltando de evento em evento.

## Cenários

Um cenário é um YAML com campos em português. Campo desconhecido é erro, para que um erro de digitação não seja ignorado sem aviso.

```yaml
nome: atual
horizonte_min: 720        # o portão recebe caminhões por 12 h (padrão: duração do perfil)
replicas: 30              # padrão 30; mínimo 2
semente: 1                # semente da primeira réplica; padrão 1
warmup_min: 60            # período inicial fora da estatística
chegadas:
  perfil_horario: [4, 12, 28, 31, 22, 15, 9, 6, 6, 8, 5, 3]   # caminhões/hora
  agendados: 0.5          # opcional: fração com hora marcada
  tolerancia_min: 15      # opcional: folga do agendado em torno do horário (padrão 15)
recursos:
  balanca_entrada: 1      # padrão 1
  balanca_saida: 1        # padrão 1; 0 = a mesma balança faz entrada e saída
  docas: 12
pesagem: {min: 3, moda: 4, max: 7}        # opcional; este é o padrão
descarga:                                 # um ou mais tipos de carga
  granel:     {min: 35, moda: 48, max: 95}
  paletizada: {min: 18, moda: 25, max: 40}
  # proporcao: 0.6 em cada tipo para mudar o mix; sem ela, divide igualmente
```

Os exemplos estão em [`exemplos/`](exemplos/). O `atual.yaml` usa 12 docas, não 3 como no mapa: com esses tempos de descarga, 3 docas atendem no máximo ~50 caminhões em 12 h, e o perfil manda ~149.

## Como funciona

O relógio não avança de minuto em minuto — ele salta direto para o instante do próximo evento agendado. O laço principal é:

1. tira o evento mais próximo de uma min-heap
2. adianta o relógio até ele
3. executa o evento, que normalmente agenda novos eventos à frente

O caminhão passa por `chegada → fila do pátio → balança de entrada → doca → balança de saída → saída`. Cada recurso (`des.Resource`) tem capacidade N e fila FIFO; quem encontra o recurso ocupado espera e é acordado por quem libera.

- **Chegadas** seguem o perfil horário como processo de Poisson não-homogêneo, gerado por *thinning*. Os agendados são espalhados em horários igualmente espaçados, com uma folga sorteada.
- **Tempos de serviço** são triangulares (mínimo, moda, máximo), e cada caminhão sorteia os seus na chegada, de um fluxo aleatório próprio.
- **Réplicas** rodam em paralelo, cada uma com sua `Engine`. O resultado é média ± intervalo de confiança de 95% (t de Student) entre réplicas.
- **Warm-up**: o que acontece antes de `warmup_min` não entra na estatística.
- **Utilização** é ponderada pelo tempo, medida entre o warm-up e o fechamento do portão.
- **Gargalo** é o recurso onde os caminhões mais esperam, em média.
- **Saturação**: se a carga oferecida de um recurso passa de 100%, o cenário é marcado como saturado em vez de mostrar uma média que só reflete o horizonte.

## Estrutura

```
cmd/simterminal/     CLI: run, compare, demo
internal/des/        motor: relógio, fila de eventos, recursos, fluxos aleatórios, thinning
internal/stats/      métricas: média, p95, média no tempo, intervalo de confiança
internal/terminal/   domínio: caminhão, chegadas, balanças, docas
internal/scenario/   leitura do YAML, réplicas, resumo, gargalo
internal/report/     tabela no terminal e CSV
exemplos/            cenários prontos
```

`des` não sabe o que é um caminhão: o mesmo motor serve para pronto-socorro ou linha de produção.

## Validação

Simulação é o tipo de software que mente com elegância: o resultado sempre parece plausível, mesmo com a lógica errada. Cada camada tem uma âncora:

| Âncora | Teste |
|---|---|
| M/M/1 bate com `Wq = ρ / (μ − λ)` | `TestMM1MatchesAnalyticFormula` |
| M/M/1 com `Resource` dá resultado **idêntico** à versão manual | `TestResourceMM1MatchesManual` |
| M/M/2 bate com Erlang C | `TestResourceMMcMatchesErlangC` |
| chegadas por hora batem com o perfil | `TestNextArrivalFollowsProfile` |
| lei de Little `L = λW`, exata | `TestLittlesLaw` |
| nenhum caminhão some | `TestNoTruckLost` |
| sem fila, tempo no terminal = soma dos serviços | `TestLightLoad` |
| IC de 95% cobre a média verdadeira ~95% das vezes | `TestMeanCICoverage` |
| meia-largura do IC cai com `1/√R` | `TestHalfWidthShrinksWithSqrtR` |
| mesmas sementes estreitam o IC da diferença | `TestCommonRandomNumbersShrinkDiffCI` |

## Roadmap

- [x] **Dia 1** — motor DES puro, validado contra M/M/1
- [x] **Dia 2** — abstração de recurso (capacidade N, fila FIFO, estatística de ocupação)
- [x] **Dia 3** — modelo do terminal: chegadas com perfil por hora, tipos de carga, balança + docas
- [x] **Dia 4** — múltiplas réplicas com sementes diferentes, média e intervalo de confiança
- [x] **Dia 5** — cenários declarados em arquivo de configuração e saída comparativa

Depois do MVP:

- [x] identificação automática de gargalo
- [x] agendamento de janelas (`agendados`)
- [ ] turnos e paradas — almoço, troca de turno, manutenção
- [ ] calibração com dado real — ajustar as distribuições a um CSV de movimentação
- [ ] visualização — fila vs. hora, ou animação do pátio

## Licença

MIT
