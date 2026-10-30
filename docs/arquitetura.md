# Orquestrator — Arquitetura (rascunho para aprovação)

Board de gerenciamento de projetos onde **fluxos desenhados visualmente** disparam **agentes de IA**
(inicialmente Claude Code via CLI) quando cards são criados ou entram em uma baia.
Expõe **API REST** e **servidor MCP** para integrações.

---

## 1. Decisões tomadas

| Tema | Decisão |
|---|---|
| Backend | Go |
| Frontend | React + ReactFlow (editor de fluxo) |
| Qualidade | 100% de cobertura (Go e React), lint nas duas frentes. Excluídos: código gerado e `main.go` de bootstrap |
| Banco e fila | PostgreSQL para dados **e** fila de jobs |
| Workspace/sessão | Diretório persistente por card, montável em qualquer worker |
| Multiusuário / auth | Fora do escopo por enquanto |
| Card × Fluxo | Entidades separadas. Um card dispara FlowRuns; o fluxo pode ramificar |
| Paralelismo | Modelo de dados suporta vários nós ativos por FlowRun; MVP executa caminho linear com desvios condicionais |
| Baias | Dinâmicas (globais ou do fluxo). Nó `MoveCard` escolhe a baia destino |
| Consumo | Histórico de tokens/turnos/custo por NodeRun, agregado por card |
| Agentes | Perfis configuráveis de Claude Code ou opencode (o tipo é do perfil; um fluxo mistura os dois). Login via `orquestrator agent login claude\|opencode` |
| Projetos | Cadastro de projetos com tipo de origem (`local` no MVP; `git` remoto no futuro). Card referencia um projeto **opcional** |
| Portabilidade | `WorkspaceProvider` plugável, caminhos fixos no sandbox (`/workspace`, `/claude`) e `jobs.requirements` para afinidade de worker — prepara Docker e multi-host sem mudar engine/contrato |
| Tipos de card | Cadastro dinâmico de tipos; gatilhos podem filtrar por tipo |
| Hierarquia | Card pode ter card pai (`parent_card_id`); eventos dos filhos acordam o fluxo do pai |
| MCPs externos | Servidores MCP cadastrados (ex.: Sourcegraph) e selecionados por nó Agent |
| Workspace do agente | `git worktree` por card, a partir do `project_path` |
| Runner do MVP | `local` (dev/testes) + `bwrap` (isolamento padrão). Docker na fase multi-host |
| Stack Go | `net/http` (roteamento stdlib) + `pgx` + `sqlc` + `goose` |
| Stack Web | Vite + TypeScript + Vitest/Testing Library + TanStack Query + Tailwind + pnpm |
| MCP | SDK oficial `modelcontextprotocol/go-sdk` |
| API | OpenAPI spec-first; servidor Go e cliente TS gerados a partir do contrato |
| Repositório | Monorepo: `backend/` + `web/` |

---

## 2. Componentes

```
                    ┌───────────────────────────────┐
  React (board,     │  orquestrator server (Go)      │
  editor de fluxo) ─┤   • API REST  (+ SSE eventos)  │
  Integrações ──────┤   • Servidor MCP               │
  Agentes (MCP) ────┤   • Engine de fluxo            │
                    └──────────────┬────────────────┘
                                   │ jobs (tabela)
                              ┌────▼─────┐
                              │ Postgres │
                              └────▲─────┘
                                   │ SKIP LOCKED + LISTEN/NOTIFY
                    ┌──────────────┴────────────────┐
                    │  orquestrator worker (Go) x N │
                    │   Runner (interface)          │
                    │    └─ claude -p ... stream-json
                    └───────────────────────────────┘

  orquestrator agent login claude   ← CLI de configuração do perfil
```

Um único binário Go com subcomandos: `server`, `worker`, `agent login`, `migrate`.

### 2.1 Camadas do backend

- **domain** — entidades e regras puras (sem I/O).
- **service** — casos de uso (criar card, mover card, responder perguntas, publicar fluxo…).
  Único ponto de entrada de regra de negócio; REST e MCP são só adaptadores sobre ele.
- **engine** — avalia o grafo do fluxo: reage a eventos (card criado/movido, nó concluído,
  perguntas respondidas) e decide quais nós executar.
- **store** — persistência Postgres.
- **queue** — fila sobre Postgres.
- **runner** — executa o agente (interface `Runner`; implementações plugáveis).
- **agent** — adaptador por tipo de agente (monta comando do Claude, interpreta `stream-json`).
- **transport/rest** (inclui o stream SSE, que reaproveita os conversores da API), **transport/mcp**.
- **events** — hub em memória que distribui as mudanças do board para o SSE; um relay repassa ao
  hub as mudanças feitas pelo worker, anunciadas por `NOTIFY orq_changes` (tipo + id; o server
  relê a entidade).

Tudo que toca o mundo externo (processos, relógio, filesystem, banco, fila) fica atrás de
interfaces para viabilizar 100% de cobertura com fakes.

---

## 3. Modelo de dados

| Tabela | Campos principais |
|---|---|
| `lanes` | id, name, position, color (baias do fluxo — `scope`/`flow_id` — adiadas) |
| `projects` | id, name, source_kind (`local`; futuro `git`), local_path (repo git), remote_url? e credential_id? (futuro) |
| `card_types` | id, name, color |
| `cards` | id, title, description, card_type_id, project_id?, parent_card_id?, lane_id, position, created_at |
| `mcp_servers` | id, name, transport (`stdio`/`http`), config jsonb, secrets (criptografados) |
| `card_events` | id, card_id, type, payload jsonb, created_at — linha do tempo do card |
| `agent_profiles` | id, kind (`claude`, `opencode`), name, credential (criptografada), status |
| `flows` | id, name, enabled, draft jsonb (rascunho editável) |
| `flow_versions` | id, flow_id, version, graph jsonb (nós com config + arestas por saída), published_at |
| `flow_runs` | id, flow_version_id, card_id, trigger, status, started_at, finished_at |
| `node_runs` | id, flow_run_id, node_key, status, iteration, input jsonb, output jsonb, session_id?, tokens_in, tokens_out, cache_read, cache_write, cost_usd, turns, duration_ms, error, summary?, structured_output?, decision? (`approved`/`rejected`), feedback? |
| `agent_sessions` | id, flow_run_id, node_key, profile_id, claude_session_id (escolhido pelo board via `--session-id`) |
| `questions` | id, card_id, node_run_id, text, answer?, answered_at? |
| `artifacts` | id, card_id, name (único por card), node_run_id (quem coletou a última versão), review_node_run_id? (a última revisão humana que o pôs em revisão) |
| `artifact_versions` | id, artifact_id, version, path, content, node_run_id |
| `artifact_messages` | id, artifact_id, version, role (`human`/`agent`), text, node_run_id? (o nó de revisão humana em que a mensagem entrou) |
| `jobs` | id, kind, payload jsonb, requirements jsonb, status, run_at, attempts, locked_by, lease_until, last_error |

Fluxos são **versionados**: um FlowRun em andamento continua na versão em que começou.

---

## 4. Nós do MVP

| Nó | Config | Saídas |
|---|---|---|
| **Trigger** | `card_created` ou `card_entered_lane(lane)`, filtro opcional por tipo de card | `next` |
| **Agent** | perfil, prompt inicial, tools permitidas/bloqueadas, MCPs externos, schema de saída (opcional), max_turns, modelo, somente leitura, gateway (+ `max_rounds`), artefatos (nome + arquivo) | `done`, `failed`; com gateway: `approved`, `rejected`, `max_rounds_exceeded`, `failed` |
| **MoveCard** | baia destino | `next` |
| **End** | — | — |

**Fase 2 (F11, feita):**

| Nó | Config | Saídas |
|---|---|---|
| **CreateCards** | baia e tipo dos cards criados; os itens vêm da lista `cards` (título, descrição, projeto pelo nome) que o nó anterior entrega | `next`, `failed` |
| **WaitChildren** | baia em que um filho conta como concluído | `all_done`, `some_failed` |
| **HumanReview** | nomes dos artefatos revisados (vazio: os coletados no run) | `approved`, `changes_requested` |

**CreateCards** lê a lista `cards` da saída estruturada do Agent anterior (o editor preenche o
schema com "Usar o schema de cards filhos") e cria todos os filhos numa transação, ou nenhum:
um projeto desconhecido ou uma lista vazia saem por `failed`. Os filhos disparam os próprios
fluxos pela baia em que nascem, e a saída do nó lista os cards criados.

**WaitChildren** olha os filhos do card: `all_done` quando todos estão na baia configurada,
`some_failed` assim que a última execução de algum filho falha ou é cancelada antes de chegar
lá (sem filhos, `all_done`). Enquanto isso o nó fica `waiting` e o FlowRun do pai `waiting`. O
pai é reavaliado quando um filho entra numa baia (job `flow.trigger` do filho), quando o fluxo
de um filho termina e quando a execução de um filho é cancelada (job `flow.child_changed`);
`SettleWait` garante que só uma dessas reavaliações siga a saída.

O `get_card` do MCP devolve os artefatos do card (última versão e o estado da revisão, `approved` depois da aprovação) e, num
card filho, o card pai com os artefatos dele: o agente que divide o trabalho e os que
implementam leem o plano aprovado.

Futuro: `PR`, `Fork/Join` (paralelo dentro do fluxo), `WaitSignal` (review entre repositórios
no fluxo do pai), outros agentes.

Saídas de nós ficam em `node_runs.output` e são acessíveis aos nós seguintes do mesmo FlowRun
(ex.: `CreateCards` lê a saída estruturada do Agent).

### 4.1 Saída como entrada e reentrada (o loop do gateway)

**A saída de um nó é a entrada do seguinte, como mensagem na sessão do agente:** a saída
estruturada em JSON quando existe, senão o texto final (no `reject`, o feedback). Na primeira
entrada a mensagem é o prompt do nó, o card e essa entrada (`## Input`); **ao entrar de novo num nó
Agent que já tem sessão neste FlowRun, a engine retoma a sessão (`--resume`) só com a entrada.**
MoveCard e End repassam a entrada que receberam.

Revisão é um nó Agent com o **gateway** ligado: o agente decide com as tools `approve(reason?)` ou
`reject(feedback)`, e o system prompt ganha a instrução de chamar uma delas. Ligando `rejected` de
volta ao dev, ele recebe o feedback na mesma sessão; `max_rounds` limita as rejeições do run. O
loop fica visível no desenho e não é um caso especial.

### 4.1.1 Artefatos e revisão humana

Um nó Agent pode declarar artefatos (nome + arquivo do workspace, ex.: "Plano" ← `plan.md`). Quando
o agente termina, o worker lê os arquivos (texto, até 1 MiB) e grava uma versão nova de cada
artefato do card; arquivo ausente falha o nó.

A revisão humana é um nó do fluxo, **HumanReview**, e o loop fica no desenho como o do gateway:

```
Agent (planner) ──done──▶ HumanReview ──approved──▶ MoveCard(Arquitetura pronta)
      ▲                         │
      └────changes_requested────┘
```

1. Ao entrar no nó, a engine põe sob revisão os artefatos configurados (por nome; sem nomes, os
   coletados neste run) e o NodeRun fica `awaiting_review`, com o FlowRun `waiting`. Sem
   artefatos para revisar, o nó falha o run.
2. Na tela de revisão (`#/review/<card>`, aberta pelo botão "Revisar" do card no board, que o
   `FlowRun.awaiting_review` acende) ou na aba Artefatos, o humano lê a última versão (Markdown)
   à esquerda e, no chat à direita, escreve o que ajustar; selecionar um trecho oferece citá-lo
   (vira uma citação Markdown na mensagem). A mensagem
   fica no chat do artefato e o nó sai por `changes_requested`; a saída leva ao agente o pedido
   (`ReviewPrompt`), e ele o recebe **na mesma sessão do run**, como o feedback de um gateway.
3. O agente atualiza o arquivo e termina; o board recoleta a versão nova e o run volta ao nó de
   revisão. A entrada que chega (a resposta do agente) vai para o chat como mensagem do agente.
4. "Aprovar" (na tela de revisão, na aba Artefatos ou no nó, na aba Execução) sai por `approved`, repassando o que o
   nó recebeu; o fluxo segue, por exemplo movendo o card para a baia que dispara o próximo fluxo.

Só a última versão aceita comentários e aprovação; as anteriores ficam para leitura. O chat só
aceita mensagens enquanto um nó de revisão espera pelo artefato: o estado da revisão (`none`,
`open`, `revising`, `approved`, `closed`) vem do nó de revisão em `artifacts.review_node_run_id`
e do run dele. Quem decide termina o NodeRun numa transação e enfileira o job `flow.continue`,
que a engine trata seguindo a saída do nó. Mover o card não muda a revisão; cancelar o run a
encerra (`closed`).

### 4.2 Perguntas do agente

1. O agente chama a tool MCP `ask_human(questions[])` e encerra o turno.
2. NodeRun → `awaiting_input`; perguntas anexadas ao card; card **não se move**.
3. Humano responde (UI, REST ou MCP).
4. Engine enfileira job de retomada: `claude --resume <session> -p "<respostas>"`.

---

## 5. Protocolo agente ↔ board (MCP)

Cada execução recebe um `--mcp-config` com o servidor MCP do board (token de execução que
identifica o NodeRun) **mais os MCPs externos selecionados no nó** (ex.: Sourcegraph), com
segredos injetados no momento da execução. Tools do board expostas ao agente:

| Tool | Uso |
|---|---|
| `ask_human(questions[])` | registra dúvidas e pausa o nó |
| `submit_output(json)` | saída estruturada, validada contra o schema configurado no nó |
| `approve(reason?)` / `reject(feedback)` | decisão de um nó com gateway |
| `report_done(summary)` | resumo final do trabalho |
| `get_card()` | contexto do card (título, descrição, histórico relevante) |
| `list_projects()` | projetos do board (id e nome, sem o caminho local): para ligar o `project_id` do card a um nome e nomear o projeto dos cards criados |

O cadastro de um MCP externo pode ser testado antes de salvar (`POST /mcp-servers/test`): o board
conecta como os agentes, lista as tools e, se pedido, chama uma tool. Alguns servidores só conferem a
credencial numa chamada (o Sourcegraph aceita o handshake e o `tools/list` com qualquer token), por
isso a tool opcional, que roda de verdade e deve ser de leitura. Sem segredos no formulário, o teste
usa os já gravados. Servidores stdio são testados no processo da API, fora do sandbox dos agentes.

Se o agente terminar sem chamar `report_done`, o resultado final do `stream-json` é usado; um nó
com gateway que termina sem `approve`/`reject` falha, e um Agent com schema sem `submit_output`
também.

O endpoint `/mcp` é stateless: requisições com `Authorization: Bearer <token do NodeRun>` (válido só
enquanto o nó roda) recebem as tools acima; sem token, recebem as tools de integração.

O agente **não comita nem abre PR** (garantido pelas tools bloqueadas/prompt; o PR será um nó futuro).

---

## 6. Execução

### 6.1 Fila (Postgres)

- Worker pega job com `SELECT … FOR UPDATE SKIP LOCKED`, grava `locked_by` e `lease_until`.
- Heartbeat renova o lease durante a execução; job com lease vencido volta para a fila.
- `LISTEN/NOTIFY` acorda workers sem polling agressivo.
- Kinds: `flow.trigger` (enfileirado na mesma transação do evento `card.created`/`card.moved`;
  dispara os fluxos cujo gatilho combina) e `agent.run` (roda o nó Agent e, em seguida, a engine
  segue a saída dele). A engine roda no worker; os nós MoveCard e End executam dentro do job que
  chega até eles.
- **Afinidade:** cada job tem `requirements` (ex.: `{"host": "x"}`, futuramente `{"runner": "docker"}`);
  cada worker anuncia suas capacidades e só pega jobs compatíveis. Projetos `local` exigem o host
  onde a pasta existe. No MVP o worker anuncia apenas o próprio host.

### 6.2 Diretórios por card

```
<data_dir>/cards/<card_id>/
  workspace/   ← git worktree do project_path (branch orq/card-<card_id>), cwd do agente
  claude/      ← casa do agente: CLAUDE_CONFIG_DIR do Claude, HOME do opencode (sessões e credenciais)
  logs/        ← stream-json bruto de cada NodeRun
```

- O worktree é criado no primeiro nó Agent do card e reutilizado por todos os nós seguintes
  (o revisor enxerga o `git diff` do mesmo worktree).
- O `local_path` do projeto precisa ser um repositório git; o cadastro do projeto valida isso.
- Card **sem projeto**: `workspace/` é um diretório vazio (agente de análise que só lê via MCP).
- Vários cards do mesmo projeto rodam sem conflito, cada um no seu worktree.

**Caminhos fixos no sandbox.** Todo runner monta `workspace/` em `/workspace` (cwd do agente) e
`claude/` em `/claude` (`CLAUDE_CONFIG_DIR=/claude`). Como as sessões do Claude são indexadas
pelo cwd, isso torna o `--resume` portável entre runners e hosts.

**Worktree dentro do sandbox.** O `.git` de um worktree aponta para o caminho absoluto do `.git`
do repositório original. O runner monta esse diretório no **mesmo caminho absoluto** dentro do
sandbox, para que `git status`/`git diff` funcionem (bwrap hoje, Docker no mesmo host amanhã).

**`WorkspaceProvider`** — a preparação do workspace é plugável:

```go
type WorkspaceProvider interface {
    Prepare(ctx context.Context, card Card, project *Project) (Workspace, error)
    Cleanup(ctx context.Context, card Card) error
}
```

| Implementação | Origem | Como prepara | Onde roda |
|---|---|---|---|
| `LocalWorktree` (MVP) | `local` | `git worktree` a partir da pasta do host | só no host da pasta |
| `Empty` (MVP) | sem projeto | diretório vazio | qualquer worker |
| `GitClone` (futuro) | `git` | espelho `--mirror` por worker + clone por card | qualquer worker/host |

**Multi-host (futuro):** `<data_dir>/cards/` em storage compartilhado (NFS / volume persistente
no Kubernetes). Com origem `git` o clone vive inteiro no volume do card; origem `local`
continua presa ao seu host via `requirements`.
- Limpeza do worktree: ao fim do fluxo (ou nó de PR futuro); política detalhada a definir.

### 6.3 Credenciais

`orquestrator agent login claude` abre o CLI do Claude num perfil isolado e orienta o
`claude setup-token` (token de longa duração). O board guarda o token criptografado e o
injeta em cada execução via `CLAUDE_CODE_OAUTH_TOKEN` (ou `ANTHROPIC_API_KEY`).
Nenhuma escrita concorrente em arquivo de credenciais compartilhado.

`orquestrator agent login opencode [--integration <id>]` copia o login do próprio opencode do
usuário (`opencode auth export`, feito depois de `opencode auth login` no provedor) para o
perfil, criptografado, sem mostrar o segredo no terminal. Antes de cada execução o executor
grava o JSON num arquivo temporário da casa do agente do card e roda `opencode auth import`
com `HOME` apontando para ela; um novo login chega assim também aos cards em andamento.

### 6.3.1 opencode

O adaptador do opencode roda `opencode run --standalone --format json --auto --agent build
--session ses_<id> [--model provedor/modelo] -- <prompt>` com `HOME` na casa do agente do card
(isola sessões, credenciais e o que o opencode lê de `~`) e a configuração da execução em
`OPENCODE_CONFIG_CONTENT`:

- `mcp`: o servidor do board (`remote`, com o token da execução) e os do nó (`local`/`remote`);
- `instructions`: um arquivo temporário com o texto do board (o equivalente ao
  `--append-system-prompt` do Claude);
- `permission`: `git commit*`/`git push*` e `websearch` negados sempre (a busca web precisa de conta
  opencode; no modo `--standalone` ela é cancelada e o `run` inteiro sai com erro), `edit` negado em
  nós somente leitura;
- `agent.build.steps`: o limite de turnos do nó.

O id da sessão é derivado do UUID guardado em `agent_sessions` (`ses_` + hex); `--session`
cria a sessão quando ela não existe, então retomar é igual a começar. O opencode não tem
evento de resultado: o resultado é o último texto, o consumo soma os `step_finish` e os
eventos `error` reprovam o nó com uma dica para os casos que o perfil resolve
(`provider.no-route`: sem credencial do provedor; `provider.free-limit`: cota gratuita, com o
tempo até liberar). Se o processo sai com erro sem evento `error`, a mensagem do nó cita a última
tool que falhou. As ferramentas permitidas/bloqueadas do nó são nomes do Claude e não valem
para o opencode.

### 6.4 Runner

```go
type Runner interface {
    Run(ctx context.Context, spec RunSpec, events chan<- Event) (Result, error)
}
```

Implementações:

- **`local`** (MVP) — processo direto; usado em dev e testes.
- **`bwrap`** (MVP, padrão) — `/usr` e `/etc` do host somente leitura; graváveis apenas
  `/workspace`, `/claude`, `/tmp` e o `.git` do repositório original (no caminho original); home privada; rede liberada (o agente precisa da API da
  Anthropic e do servidor MCP do board).
- **`docker`** (futuro) — imagem com toolchains, limites de recurso, políticas de rede.

### 6.5 Consumo

O evento `result` do `stream-json` traz usage (tokens, cache), `num_turns`, `duration_ms` e
`total_cost_usd`. Gravado em `node_runs` e agregado por card. Com token de assinatura o custo
em dólar é estimativa.

---

## 7. Interfaces externas

- **REST** — boards/lanes/cards/flows/runs/questions/agent-profiles. Contrato OpenAPI.
- **SSE** — eventos em tempo real (card movido, saída do agente, perguntas, status de nós).
- **MCP** — tools para integrações (`list_cards`, `create_card`, `move_card`,
  `answer_questions`, `list_flows`, `get_run`…) e as tools de agente da seção 5.
- **Webhooks de saída** — fora do MVP.

---

## 8. Frontend

- **Board** — baias, cards, drag & drop, painel do card (descrição, linha do tempo,
  perguntas pendentes com campo de resposta, decisões de gateway, artefatos com chat de revisão,
  consumo, saída do agente ao vivo).
- **Editor de fluxo** — ReactFlow; paleta de nós, painel de configuração do nó selecionado,
  validação do grafo, publicar versão.
- **Baias** — CRUD de baias globais e do fluxo.

---

## 9. Qualidade

- Go: `go test -race -coverprofile`, gate de 100% no CI (excluindo gerados e `cmd/*/main.go`), `golangci-lint`.
- React: testes unitários com cobertura 100% (gate), ESLint.
- Toolchains gerenciadas via `mise.toml` (Go, Node, Postgres para dev se aplicável).

---

## 10. Escopo do MVP

**Dentro**
1. CRUD de baias, projetos, tipos de card e cards (projeto opcional, card pai).
2. Editor de fluxo com Trigger, Agent (com gateway e artefatos), MoveCard, End; versionamento.
3. Engine linear com desvios condicionais e reentrada (loop do gateway).
4. Worker + fila Postgres + Runner do MVP.
5. Perguntas do agente, saída estruturada e retomada de sessão.
6. Cadastro de MCPs externos e seleção por nó Agent.
7. Histórico de consumo por card.
8. API REST + SSE + MCP.
9. `agent login claude` (e `agent login opencode`, F13).

**Fase 2**
Nós `HumanReview`, `CreateCards`, `WaitChildren` — habilitam o cenário de referência (seção 11).

**Fora (próximas fases)**
Nó de PR, review entre repositórios no fluxo do pai (`WaitSignal`), paralelismo Fork/Join,
projetos com origem `git` (`GitClone`), runner Docker, storage compartilhado entre hosts, multiusuário/auth, outros agentes, webhooks, orçamento/limites de custo,
runners multi-host.

---

## 11. Cenário de referência: proposta multi-repo

Um agente com acesso ao Sourcegraph analisa o código, propõe mudanças em vários repositórios,
um humano aprova, e cada repositório ganha um card implementado por outro agente.

**Fluxo "Proposta"** — gatilho: card criado do tipo `proposta` (sem projeto)
```
Trigger → Agent (MCP Sourcegraph; submit_output: [{project, plano}])
        → HumanReview ──changes_requested──▶ Agent (retoma sessão com o pedido)
            │approved
        → CreateCards (um card filho `implementação` por item, com projeto)
        → WaitChildren ──all_done──▶ MoveCard(Concluído)
```

**Fluxo "Implementação"** — gatilho: card criado do tipo `implementação`
```
Trigger → Agent (dev, worktree do projeto) ⇄ Agent (gateway, somente leitura) → [PR, futuro] → MoveCard(Pronto)
```

O agente referencia repositórios pelo nome; o cadastro de projetos mapeia esse nome para a pasta
local. Cards filhos rodam em paralelo naturalmente (um FlowRun e um worktree por card).

---

## 12. Decisões em aberto

- [x] Estratégia de workspace — git worktree por card
- [x] Runner do MVP — local + bwrap
- [x] Stack Go — net/http + pgx + sqlc + goose
- [x] Stack frontend — Vite + TS + TanStack Query + Tailwind + pnpm
- [x] SDK MCP em Go — SDK oficial `modelcontextprotocol/go-sdk`
- [x] OpenAPI — spec-first com geração de código (servidor Go e cliente TS)
- [x] Estrutura do repositório — monorepo `backend/` + `web/`
