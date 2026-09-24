# simterminal — plano do projeto

Simulador de eventos discretos (DES) para terminais de carga: portos, armazéns de grãos, centros de distribuição.

---

## 1. O problema

Num terminal de carga o gargalo raramente é o que parece.

O gerente vê uma fila de três horas no pátio e conclui que falta doca. Manda comprar equipamento, contratar gente, ampliar área. Seis meses depois a fila continua — porque o gargalo real era a balança única de quatro minutos, ou o pico de chegadas concentrado entre 7h e 9h que nenhuma quantidade de doca resolve.

O motivo do erro é estrutural: **sistemas com fila não são intuitivos**. O tempo de espera não cresce de forma proporcional à carga, cresce de forma explosiva perto da saturação. Sair de 80% para 90% de utilização não piora a fila em 12% — pode dobrá-la. Nenhuma planilha captura isso, porque planilha calcula média e o problema está na variabilidade.

Testar hipótese na operação real custa uma safra inteira e arrisca contrato. No simulador custa trinta segundos.

## 2. O que o projeto responde

- Quanto tempo médio um caminhão fica hoje dentro do terminal?
- Se eu abrir uma segunda balança, a fila cai quanto? E uma terceira doca?
- Qual recurso é realmente o gargalo?
- Quantos caminhões por dia esse terminal aguenta antes de virar caos?
- E se metade dos caminhões vier com hora agendada?

O entregável final não é um gráfico bonito, é uma tabela de decisão:

```
Cenário               Tempo médio      p95      Fila máx   Util. balança
Atual (1 bal, 3 doc)  2h47 ± 11min    5h02        31          97%
+ 1 balança           1h12 ± 06min    2h20        14          61%
+ 1 doca              2h39 ± 10min    4h51        29          96%
```

Aqui a conclusão salta: a doca extra não resolve nada, a balança sim.

## 3. Como funciona

### O relógio salta

A ideia central da simulação por eventos discretos é que **o tempo não avança de minuto em minuto — ele salta direto para o instante do próximo evento agendado**.

Existe uma lista ordenada por tempo (uma min-heap) com as coisas que vão acontecer:

```
07:00  caminhão 1 chega
07:04  caminhão 2 chega
07:09  caminhão 1 sai da balança
07:12  caminhão 3 chega
```

O laço principal é só isto:

1. retira o evento mais próximo da heap
2. adianta o relógio até o instante dele
3. executa o evento — que normalmente agenda novos eventos à frente

Quando "caminhão 1 chega" executa, ele pergunta se a balança está livre. Se estiver, ocupa e agenda "caminhão 1 sai da balança" para daqui a quatro minutos. Se não estiver, entra na fila e **não agenda nada** — quem vai acordá-lo é o caminhão da frente, ao liberar a balança.

Um dia inteiro de operação roda em milissegundos porque todo intervalo em que nada acontece é pulado.

### O fluxo do caminhão

```
Chegada  →  Fila do pátio  →  Balança entrada
                                     ↓
Saída    ←  Balança saída   ←     Descarga
```

Em cada etapa o caminhão ou é atendido na hora, ou entra numa fila esperando o recurso liberar. Cada transição registra um marco de tempo, o que permite reconstruir depois onde exatamente cada caminhão perdeu o dia.

## 4. Arquitetura

```
cmd/simterminal/     CLI: run, compare
internal/des/        motor: relógio, fila de eventos, recursos, distribuições
internal/stats/      métricas: média, p95, máximo, intervalo de confiança
internal/terminal/   domínio: caminhão, chegadas, balança, docas
internal/scenario/   leitura de configuração e execução de réplicas
```

A separação que importa: `des` **não sabe o que é um caminhão**. É um motor de simulação genérico, que serve para terminal, para pronto-socorro ou para linha de produção. O domínio vive em `terminal` e conversa com o motor apenas agendando eventos e pedindo recursos.

Isso não é purismo: é o que permite que o mesmo motor, daqui a um mês, vire o simulador de tráfego ou o de ônibus sem reescrever nada.

## 5. Roadmap

Cada etapa entrega algo que **roda e é verificável**. Nada de escrever o modelo inteiro e testar no fim — simulação é o tipo de software que mente com elegância: o resultado sempre parece plausível, mesmo com a lógica errada. Por isso cada etapa tem uma âncora: uma fórmula fechada, uma lei estatística ou um teste de sanidade.

### Dia 1 — motor de eventos ✅

Relógio virtual, min-heap de eventos com desempate por ordem de agendamento, distribuições (exponencial, uniforme, triangular), acumulador de métricas, CLI de demonstração.

**Âncora:** fila M/M/1 com 300 mil clientes batendo com a fórmula fechada `Wq = ρ / (μ − λ)` dentro de 5%.

### Dia 2 — abstração de recurso

Hoje a lógica de "está ocupada? entra na fila?" está espalhada no `main.go`. Isso não escala para portaria + balança + N docas.

Construir um tipo `Resource` com capacidade N e fila FIFO:

```go
r := des.NewResource(e, "balança", 1)
r.Request(func(release func()) {
    e.ScheduleIn(tempoPesagem, "fim pesagem", release)
})
```

Quem chama `Request` ou é atendido na hora, ou fica na fila; o callback dispara quando chegar a vez. `release` devolve o recurso e puxa o próximo.

Métricas ficam dentro do recurso: espera por entidade, fila máxima e utilização **ponderada pelo tempo** — não é "atendidos ÷ chegados", é a área sob a curva de ocupação dividida pelo horizonte. Essa distinção derruba muito relatório por aí.

**Âncora:** reescrever o teste M/M/1 usando `Resource` e exigir o mesmo resultado. Deu diferente, o recurso tem bug.

### Dia 3 — o modelo do terminal

Entidade:

```go
type Caminhao struct {
    ID        int
    TipoCarga string
    ChegadaEm float64
    Marcos    map[string]float64
}
```

**Chegadas com perfil por hora.** Caminhão não chega uniformemente: tem pico às 7h e deserto às 14h. Isso é um processo de Poisson não-homogêneo, e a forma correta de gerar é por *thinning* — sorteia na taxa máxima e rejeita com probabilidade `1 − λ(t)/λmax`. São seis linhas de código e evita o erro clássico de "trocar a média a cada hora", que distorce o comportamento justamente nas viradas, onde está o pico.

**Tempos de serviço.** Sem histórico, a triangular é a melhor escolha: casa com o que o operador sabe dizer — "leva uns 48 minutos, no melhor caso 35, no pior 95".

**Âncoras:**

- **Lei de Little:** `L = λ × W`. O número médio de caminhões dentro do terminal tem que bater com a taxa de chegada vezes o tempo médio de permanência. Vale para qualquer sistema estável e é o melhor detector de erro de contabilidade que existe.
- Longe da saturação, throughput ≈ chegadas. Sumiu caminhão, tem bug.

### Dia 4 — réplicas e honestidade estatística

Uma rodada é **uma amostra**, não uma resposta. Duas sementes diferentes dão 2h10 e 3h05 para o mesmo cenário, e quem olhar só uma tira conclusão errada com total convicção.

- R réplicas com sementes distintas, cada uma com sua própria `Engine` — nada compartilhado, então paraleliza com goroutines de graça
- descarte de warm-up: o terminal começa vazio, o que nenhum terminal real está; os primeiros X minutos não entram na estatística
- média e **intervalo de confiança de 95%** via t de Student

A saída deixa de ser `2h47` e passa a ser `2h47 ± 11min`. Só assim "o cenário A é melhor que o B" vira afirmação defensável.

**Truque que vale ouro:** ao comparar cenários, use *common random numbers* — as mesmas sementes nos dois. A diferença observada passa a vir da mudança estrutural, não do sorteio, e o intervalo de confiança da diferença encolhe muito.

**Âncora:** a meia-largura do IC tem que cair proporcional a `1/√R`. Quadruplicou as réplicas, o erro caiu pela metade.

### Dia 5 — cenários e comparação

Configuração declarativa, sem recompilar:

```yaml
nome: atual
horizonte_min: 720
replicas: 30
warmup_min: 60
chegadas:
  perfil_horario: [4, 12, 28, 31, 22, 15, 9, 6, 6, 8, 5, 3]
recursos:
  balanca_entrada: 1
  balanca_saida: 1
  docas: 3
descarga:
  granel:     {min: 35, moda: 48, max: 95}
  paletizada: {min: 18, moda: 25, max: 40}
```

Comandos:

```bash
simterminal run -c atual.yaml
simterminal compare atual.yaml maisbalanca.yaml maisdoca.yaml
```

Saída em tabela no terminal e CSV para planilha.

## 6. Depois do MVP

Em ordem de retorno pelo esforço:

1. **Identificação automática de gargalo** — o recurso com maior utilização e maior fila, com uma linha de conclusão em texto no fim do relatório.
2. **Agendamento de janelas** — o cenário mais valioso de todos. "E se 50% vier com hora marcada?" costuma ganhar da compra de equipamento por larga margem, e é o resultado que justifica o projeto existir.
3. **Turnos e paradas** — almoço, troca de turno, manutenção. O terminal não opera liso doze horas.
4. **Calibração com dado real** — ler CSV de movimentação e ajustar as distribuições ao histórico. É o que separa brinquedo de ferramenta.
5. **Visualização** — animação do pátio ao longo do dia, ou só fila vs. hora. Vende o resultado muito melhor que tabela.

## 7. Armadilhas

- **Nunca usar `rand` global.** Todo sorteio sai de `e.Rand()`. Se escapar um, adeus reprodutibilidade — e vai embora uma tarde tentando entender por que o mesmo comando dá resultados diferentes.
- **Média esconde o caso ruim.** Média de 40 min com p95 de 3h é uma operação onde motorista perde o dia com frequência. Reporte os dois, sempre.
- **Sistema saturado não converge.** Se a taxa de chegada passa a capacidade, a fila cresce indefinidamente e a "média" vira função do horizonte escolhido, não do sistema. Detecte e avise em vez de imprimir um número sem sentido.
- **Precisão falsa.** `2h47min13s` com IC de ±20 min é teatro. Arredonde ao que o dado suporta.
- **Warm-up esquecido.** Simulação que começa vazia subestima fila sistematicamente. É o viés mais comum e o mais silencioso.

## 8. Glossário

| Termo | O que é |
|---|---|
| DES | Discrete Event Simulation — o relógio salta de evento em evento |
| Entidade | O que flui pelo sistema; aqui, o caminhão |
| Recurso | O que é disputado e tem capacidade limitada; balança, doca |
| λ (lambda) | Taxa de chegada — caminhões por minuto |
| μ (mu) | Taxa de atendimento — caminhões por minuto por servidor |
| ρ (rho) | Utilização, λ/μ. Acima de 1 o sistema não é estável |
| M/M/1 | Fila com chegadas e atendimentos exponenciais e um servidor; tem solução analítica, por isso serve de teste |
| Lei de Little | `L = λ × W`; relação universal entre população, taxa e tempo |
| Warm-up | Período inicial descartado, enquanto o sistema ainda enche |
| Réplica | Uma rodada completa com uma semente; a resposta é o conjunto delas |

## 9. Licença

MIT