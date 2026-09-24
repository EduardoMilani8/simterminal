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

## Portos: fila de navios com dados reais

`simterminal porto` troca a simulação com números inventados por um **replay do ano real**. Os dados são as atracações públicas da [ANTAQ](https://www.gov.br/antaq/pt-br/central-de-conteudos/publicacoes-da-antaq/estatisticos-aquaviarios) (Estatístico Aquaviário): para cada navio, a hora em que chegou ao fundeadouro, atracou, começou e terminou de operar e desatracou, em cada berço de cada porto do Brasil.

Nada é sorteado. As chegadas, a ordem em que os navios atracaram (o *line-up* do porto) e o tempo de cada um no berço vêm do dado. O cenário muda só o que ele diz mudar — um berço a mais, operação mais rápida, navio chamado na hora certa — e o resto do ano acontece como aconteceu.

### Rodando

```bash
simterminal porto baixar                       # ~20 MB: atracações de 2022 a jun/2025
simterminal porto baixar -carga                # + tabela de carga (~400 MB/ano), para saber a mercadoria
simterminal porto bercos -porto Santos         # berços de um porto e o que cada um movimenta
simterminal porto diagnostico -c portos/paranagua.yaml
simterminal porto cenarios    -c portos/paranagua.yaml
simterminal porto capacidade  -c portos/paranagua.yaml -meta 5d
simterminal porto previsao    -c portos/paranagua.yaml
simterminal porto chegada     -c portos/paranagua.yaml -grupo "Fertilizantes"
```

O site da ANTAQ bloqueia download automático (Cloudflare). `baixar` usa o espelho dos mesmos arquivos publicado com Pacheco et al., *Methodological Pitfalls in Predicting Ship Turnaround Time at Brazilian Ports*, IEEE T-ITS 2026 ([doi:10.5281/zenodo.20549161](https://doi.org/10.5281/zenodo.20549161), CC-BY-4.0), lendo só as tabelas necessárias de dentro do zip de 5,5 GB. Quem baixar à mão do site da ANTAQ pode apontar `dados:` no YAML para a pasta: o formato é o mesmo.

### O arquivo do porto

```yaml
porto: Paranaguá                  # como a ANTAQ grafa em "Porto Atracação"
dados: ../dados/antaq
anos: [2022, 2023, 2024, 2025]
periodo: {inicio: 2024-01-01, fim: 2025-01-01}   # o que se analisa
treino:  {inicio: 2023-01-01, fim: 2024-01-01}   # onde a previsão escolhe o método
# custo_navio_dia_usd: 25000      # opcional: põe a espera em dinheiro
grupos:                           # berços que disputam os mesmos navios
  - nome: Corredor Leste (grãos)
    bercos: [PNG0212, PNG0213, PNG0214]
cenarios:
  - nome: + 1 berço
    alocacao: compartilhada       # por_berco (padrão) | compartilhada | fixa
    bercos_extra: 1
  - nome: operação 10% mais rápida
    operacao: 0.9                 # multiplica o tempo operando
  - nome: metade do tempo ocioso no berço
    ocioso: 0.5                   # multiplica o tempo atracado sem operar
```

`simterminal porto bercos` mostra a carga de cada berço para montar os grupos.

### O que Paranaguá mostrou em 2024

```
  grupo                    berços  navios  espera mediana  p90     navio-dias  ocupação  berço cheio*  ultrapassados
  Corredor Leste (grãos)   3       315     11,9 d          28,4 d  4.365       91%       79%           62%
  Fertilizantes            3       204     14,3 d          37,4 d  3.497       92%       89%           44%
  Açúcar (204)             1       141     16,3 d          27,5 d  2.397       92%       92%           43%
  Contêineres (TCP)        2       792     20 h            3,9 d   4.331       82%       70%           41%
  Granéis líquidos         2       266     2,0 d           10,3 d  1.109       37%       1%            42%
```

- **Grãos, fertilizantes e açúcar esperam por berço.** Ocupação acima de 90% e, durante a espera, todos os berços cheios em 80–90% do tempo. Aqui mais capacidade reduz a fila.
- **Nos granéis líquidos a espera não é falta de berço.** Mediana de 2 dias com os berços livres em 99% desse tempo: o navio espera carga, tanque ou documento. Um berço a mais não mudaria nada — o tipo de conclusão que só o dado mostra.
- **A fila não é por ordem de chegada.** 62% dos navios de grãos viram alguém que chegou depois atracar antes. Muitos chegam antes de a carga estar pronta, e o dado não diz quando ficou. Por isso todo cenário sai como faixa entre duas leituras (abaixo).
- **Fila virtual reduz metade do fundeio sem atrasar a atracação.** Com a reserva certa (ver `chegada`), o tempo parado no fundeadouro cai 54% nos fertilizantes, 67% nos grãos e 75% no açúcar, e a atracação atrasa de 2 a 4 horas em média.
- **Marcar hora pela previsão não funciona.** A melhor previsão de espera erra 3 a 6 dias (mediana) em esperas de 12 a 16. Navio que chega na hora prevista atrasa a própria atracação em 5 a 13 dias.

### Os comandos

| Comando | Pergunta | Como responde |
|---|---|---|
| `diagnostico` | Qual o problema de cada grupo de berços? | Só o dado, sem simular: espera, ocupação, **berço cheio** (fração da espera com todos os berços ocupados — separa fila de berço de espera por outro motivo), ultrapassagens, tempo ocioso no berço. |
| `cenarios` | E se abrir um berço, fizer fila única, operar mais rápido, cortar o tempo ocioso? | Replay com a mudança, comparado com o mesmo replay sem mudança. |
| `capacidade` | Quantos navios por mês dá para aceitar na agenda sem a espera passar da meta? | Os mesmos navios chegando mais juntos ou mais espaçados, até a espera mediana cruzar a meta. Mostra a curva: a espera explode perto do limite. |
| `previsao` | Quanto este navio vai esperar, dito na hora em que chega? | Três métodos (mediana recente, fila ÷ vazão, regressão móvel de 12 meses), só com o que se sabia na chegada. O método de cada grupo é escolhido no treino e medido no período. |
| `chegada` | Vale instruir o navio a não vir para o fundeadouro? | Hora marcada pela previsão contra fila virtual com chamada (o pátio regulador dos navios), no replay por berço. |

O quinto problema da lista original — dimensionar equipe por turno — não está nos dados da ANTAQ. O mais perto que o dado permite é o tempo atracado sem operar (esperando começar e esperando sair), que é o cenário `ocioso`.

### Como o replay funciona

- **Estado inicial real.** O período começa com quem estava de fato no berço e no fundeadouro naquele instante. Começar com a fila vazia subestima a espera por meses: com berço a 90%, a folga para desfazer uma fila acumulada é de 10% da capacidade.
- **Posições por berço.** O menor número de navios simultâneos que cobre 98% do tempo ocupado: 1 na maioria, 2 nos píeres com dois lados. O máximo não serve: uma sobreposição de minutos, erro de registro, dobraria o berço.
- **Manobra e prontidão.** Entre um navio sair e o próximo atracar há canal, maré e prático — o intervalo típico com fila é de 2 h. Até o percentil 90 desse intervalo, conta como manobra e se repete em qualquer cenário; acima, o navio não estava pronto.
- **Duas leituras de prontidão.** *Otimista*: o navio estava pronto ao chegar. *Conservadora*: só depois de atracar o último que o ultrapassou. O efeito real fica entre as duas. Com a leitura conservadora, o modelo por berço fica a menos de 5% da espera média real na maioria dos grupos (Leste 329 h contra 333 h; TCP 131 h contra 131 h); o maior desvio é no açúcar, 16% abaixo.
- **Alocação.** `por_berco` (padrão): cada navio no berço em que atracou, e o berço atende o primeiro pronto da sua fila na ordem do *line-up*. `compartilhada`: fila única, qualquer berço do grupo. `fixa`: cada berço repete a sequência real à risca — reproduz o ano exatamente, mas só vale para mudanças pequenas: esticando o tempo, o berço fica esperando um navio que agora chega semanas depois dos outros.

### Limites

- O dado não diz quando o navio ficou pronto; daí as faixas. Faixa larga não é defeito do modelo, é o tamanho do que não se sabe.
- A previsão erra em dias, não em horas. Serve para planejar, não para marcar hora.
- Grupos de berços são decisão de quem conhece o porto: `compartilhada` supõe que qualquer navio do grupo cabe em qualquer berço dele (calado, comprimento, equipamento).
- Custo de espera, frete e combustível não estão no dado. `custo_navio_dia_usd` é do usuário.

## Estrutura

```
cmd/simterminal/     CLI: run, compare, demo, porto
internal/des/        motor: relógio, fila de eventos, recursos, fluxos aleatórios, thinning
internal/antaq/      leitura dos arquivos da ANTAQ e download do espelho
internal/porto/      navios, replay, diagnóstico, capacidade, previsão, chegada
internal/stats/      métricas: média, p95, média no tempo, intervalo de confiança
internal/terminal/   domínio: caminhão, chegadas, balanças, docas
internal/scenario/   leitura do YAML, réplicas, resumo, gargalo
internal/report/     tabela no terminal e CSV
exemplos/            cenários prontos
portos/              arquivos de porto (Paranaguá)
dados/               dados baixados (fora do git)
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
| replay sem mudança reproduz **exatamente** cada navio de Paranaguá 2024, nas duas leituras | `TestReplayReproduzOReal` |
| berço a mais e operação mais rápida nunca pioram a espera | `TestBercoExtraNaoPiora`, `TestOperacaoMaisRapidaNaoPiora` |
| em qualquer cenário, ninguém atraca antes de chegar nem passa das posições do berço | `TestInvariantesDoReplay` |
| fila virtual sem limite e sem antecedência é idêntica a não ter fila virtual | `TestChamadaSemLimiteEhOMesmoQueSemChamada` |
| a previsão na chegada não muda quando o futuro muda | `TestPrevisaoNaoOlhaOFuturo` |
| sobreposição de minutos no registro não vira mais uma posição | `TestSobreposicaoCurtaNaoViraPosicao` |
| leitor aceita BOM, CRLF, `;` entre aspas e "Valor Discrepante" | `TestCarregar` |

Os testes com dados de Paranaguá rodam se `dados/antaq` existir e são pulados sem ele. A CLI é testada de ponta a ponta com um porto sintético no formato da ANTAQ (`TestCLIPorto`).

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
- [x] dado real — replay de portos com as atracações da ANTAQ (`simterminal porto`)
- [ ] calibração do terminal de caminhões com dado real — ajustar as distribuições a um CSV de balança
- [ ] visualização — fila vs. hora, ou animação do pátio

## Licença

MIT
