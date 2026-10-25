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
| Agentes | Perfis configuráveis; só Claude no início. Login via `orquestrator agent login claude` |
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
- **events** — hub em memória que distribui as mudanças do board para o SSE.

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
| `agent_profiles` | id, kind (`claude`), name, credential (criptografada), status |
| `flows` | id, name, enabled, draft jsonb (rascunho editável) |
| `flow_versions` | id, flow_id, version, graph jsonb (nós com config + arestas por saída), published_at |
| `flow_runs` | id, flow_version_id, card_id, trigger, status, started_at, finished_at |
| `node_runs` | id, flow_run_id, node_key, status, iteration, input jsonb, output jsonb, session_id?, tokens_in, tokens_out, cache_read, cache_write, cost_usd, turns, duration_ms, error |
| `agent_sessions` | id, flow_run_id, node_key, profile_id, claude_session_id (escolhido pelo board via `--session-id`) |
| `questions` | id, card_id, node_run_id, text, answer?, answered_at? |
| `review_comments` | id, card_id, node_run_id, round, file?, line?, body |
| `jobs` | id, kind, payload jsonb, requirements jsonb, status, run_at, attempts, locked_by, lease_until, last_error |

Fluxos são **versionados**: um FlowRun em andamento continua na versão em que começou.

---

## 4. Nós do MVP

| Nó | Config | Saídas |
|---|---|---|
| **Trigger** | `card_created` ou `card_entered_lane(lane)`, filtro opcional por tipo de card | `next` |
| **Agent** | perfil, prompt inicial, tools permitidas/bloqueadas, MCPs externos, schema de saída (opcional), max_turns, modelo | `done`, `failed` |
| **Review** | perfil, prompt de review, tools, `max_rounds` | `approved`, `changes_requested`, `max_rounds_exceeded` |
| **MoveCard** | baia destino | `next` |
| **End** | — | — |

**Fase 2 (logo após o MVP):**

| Nó | Config | Saídas |
|---|---|---|
| **Approval** | texto/campos exibidos ao humano (ex.: saída estruturada do nó anterior) | `approved`, `rejected` (com comentário) |
| **CreateCards** | origem dos itens (saída estruturada), tipo de card, baia inicial, mapeamento item → título/descrição/projeto | `next` |
| **WaitChildren** | condição de conclusão dos filhos (baia alcançada ou fluxo encerrado) | `all_done`, `some_failed` |

Futuro: `PR`, `Fork/Join` (paralelo dentro do fluxo), `WaitSignal` (review entre repositórios
no fluxo do pai), outros agentes.

Saídas de nós ficam em `node_runs.output` e são acessíveis aos nós seguintes do mesmo FlowRun
(ex.: `CreateCards` lê a saída estruturada do Agent).

### 4.1 Regra de reentrada (o loop de review)

A aresta `changes_requested` do Review aponta de volta para o nó Agent de desenvolvimento.
Regra genérica da engine: **ao entrar num nó Agent que já tem sessão neste FlowRun, a engine
retoma a sessão (`--resume`) usando a saída do nó anterior (os comentários) como prompt.**
Assim o loop fica visível no desenho e não é um caso especial.

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
| `submit_review(approved, comments[])` | saída estruturada do revisor |
| `report_done(summary)` | resumo final do trabalho |
| `get_card()` | contexto do card (título, descrição, histórico relevante) |

Se o agente terminar sem chamar `report_done`/`submit_review`, o resultado final do
`stream-json` é usado; para Review sem `submit_review` o nó falha.

O agente **não comita nem abre PR** (garantido pelas tools bloqueadas/prompt; o PR será um nó futuro).

---

## 6. Execução

### 6.1 Fila (Postgres)

- Worker pega job com `SELECT … FOR UPDATE SKIP LOCKED`, grava `locked_by` e `lease_until`.
- Heartbeat renova o lease durante a execução; job com lease vencido volta para a fila.
- `LISTEN/NOTIFY` acorda workers sem polling agressivo.
- Kinds: `agent.start`, `agent.resume`, `engine.advance`.
- **Afinidade:** cada job tem `requirements` (ex.: `{"host": "x"}`, futuramente `{"runner": "docker"}`);
  cada worker anuncia suas capacidades e só pega jobs compatíveis. Projetos `local` exigem o host
  onde a pasta existe. No MVP o worker anuncia apenas o próprio host.

### 6.2 Diretórios por card

```
<data_dir>/cards/<card_id>/
  workspace/   ← git worktree do project_path (branch orq/card-<card_id>), cwd do agente
  claude/      ← CLAUDE_CONFIG_DIR da execução (sessões usadas pelo --resume)
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
  perguntas pendentes com campo de resposta, comentários de review, consumo, saída do agente ao vivo).
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
2. Editor de fluxo com Trigger, Agent, Review, MoveCard, End; versionamento.
3. Engine linear com desvios condicionais e reentrada (loop de review).
4. Worker + fila Postgres + Runner do MVP.
5. Perguntas do agente, saída estruturada e retomada de sessão.
6. Cadastro de MCPs externos e seleção por nó Agent.
7. Histórico de consumo por card.
8. API REST + SSE + MCP.
9. `agent login claude`.

**Fase 2**
Nós `Approval`, `CreateCards`, `WaitChildren` — habilitam o cenário de referência (seção 11).

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
        → Approval ──rejected──▶ Agent (retoma sessão com o comentário)
            │approved
        → CreateCards (um card filho `implementação` por item, com projeto)
        → WaitChildren ──all_done──▶ MoveCard(Concluído)
```

**Fluxo "Implementação"** — gatilho: card criado do tipo `implementação`
```
Trigger → Agent (dev, worktree do projeto) ⇄ Review → [PR, futuro] → MoveCard(Pronto)
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
