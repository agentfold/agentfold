# Orquestrator — Plano de implementação do MVP

Base: [arquitetura.md](arquitetura.md). Cada fase termina com testes a 100%, lint limpo e um commit.

## Ferramentas

| Área | Escolha |
|---|---|
| Toolchains e tarefas | `mise.toml` (Go, Node, pnpm, golangci-lint, sqlc, goose, oapi-codegen) + mise tasks |
| HTTP Go | `net/http` + `oapi-codegen` (strict server) gerado a partir de `api/openapi.yaml` |
| Banco | `pgx` + `sqlc` + `goose` |
| Testes de banco | `testcontainers-go` (Postgres efêmero por pacote de teste) |
| Dev local | `docker compose` com Postgres |
| MCP | `modelcontextprotocol/go-sdk` |
| Cliente TS | `openapi-typescript` (tipos) + `openapi-fetch`; hooks TanStack Query escritos à mão |
| Web | Vite + TS + Vitest/Testing Library + TanStack Query + Tailwind + ReactFlow + dnd-kit + pnpm |
| Lint | `golangci-lint` (config versionada) e ESLint (flat config) + Prettier |
| Criptografia do token do agente | AES-GCM da stdlib, chave via `ORQ_SECRET_KEY` |

Excluídos da cobertura: código gerado (`sqlc`, `oapi-codegen`, `openapi-typescript`) e `cmd/*/main.go`.

## Estrutura

```
orquestrator/
  mise.toml               # toolchains + tasks
  compose.yaml            # Postgres de dev
  api/openapi.yaml        # contrato (fonte da verdade)
  docs/
  backend/
    cmd/orquestrator/     # main: server | worker | agent login | migrate
    internal/
      domain/  service/  engine/  store/ (sqlc)  queue/
      runner/ (local, bwrap)  agent/claude/
      transport/{rest,sse,mcp}/  config/  crypto/
    migrations/
  web/
    src/{api,board,flows,cards,components}/
```

## Fases

### F0 — Fundação
- `git init`, `.gitignore`, `mise.toml` com toolchains e tasks (`build`, `test`, `lint`, `generate`, `cover:check`).
- Esqueleto `backend/` (módulo Go, `cmd/orquestrator` com subcomandos vazios) e `web/` (Vite + TS).
- Configs de `golangci-lint`, ESLint, Prettier.
- Gates de cobertura: script Go que falha abaixo de 100% (filtrando gerados) e threshold 100% no Vitest.
- **Pronto quando:** `mise run lint` e `mise run test` passam nas duas frentes com o gate ativo.

### F1 — Banco e domínio base
- `compose.yaml`, migrations goose: `lanes`, `projects`, `card_types`, `cards`
  (projeto opcional, `parent_card_id`), `card_events`.
- Queries sqlc, store, domínio e services de baias, projetos (`source_kind = local`, validação de
  `local_path` como repo git), tipos de card e cards.
- Testes de store com testcontainers.
- **Pronto quando:** CRUD de baias, projetos, tipos e cards coberto por testes de service e store.

### F2 — API REST e SSE
- `api/openapi.yaml` com baias, projetos, tipos de card e cards; geração `oapi-codegen`; handlers sobre os services.
- Hub de eventos SSE (card criado/movido/atualizado).
- Subcomando `server` e `migrate`.
- **Pronto quando:** API sobe, contrato validado por testes de handler.

### F3 — Web: board
- Cliente gerado, hooks TanStack Query, layout com Tailwind.
- Board com baias e cards, drag & drop (dnd-kit), criação/edição de card (tipo, projeto opcional,
  card pai visível), CRUD de baias, projetos e tipos de card.
- Atualização ao vivo via SSE.
- **Pronto quando:** board funcional contra a API real, 100% de cobertura.

### F4 — Fluxos
- Migrations `flows`, `flow_versions`; endpoints de fluxo; validação do grafo no backend
  (um Trigger, arestas válidas por tipo de saída, nós alcançáveis).
- Editor ReactFlow: paleta (Trigger, Agent, Review, MoveCard, End), painel de configuração por nó
  (filtro por tipo no Trigger; schema de saída no Agent), salvar rascunho,
  publicar versão.
- Decidido: um rascunho por fluxo + versões imutáveis ao publicar; o grafo só é validado ao
  publicar (o rascunho aceita grafo incompleto e devolve os problemas); baias continuam só
  globais (baias do fluxo adiadas até haver caso de uso).
- **Pronto quando:** fluxo desenhado, validado e publicado com versão.

### F5 — Perfis de agente, fila, worker e runner
- Migrations `agent_profiles`, `mcp_servers`, `jobs`, `agent_sessions`, `node_runs`.
- `orquestrator agent login claude`: abre o CLI num perfil isolado, guia o `setup-token`, grava token criptografado.
- Cadastro de MCPs externos (API + tela), segredos criptografados; montagem do `--mcp-config`
  com o MCP do board + MCPs do nó.
- Nó Agent e Review ganham os seletores de perfil e (Agent) de MCPs externos no editor.
- Card sem projeto: workspace vazio em vez de worktree.
- Fila Postgres (SKIP LOCKED, lease/heartbeat, LISTEN/NOTIFY, filtro por `requirements`) e
  subcomando `worker` anunciando o próprio host.
- `WorkspaceProvider` com `LocalWorktree` e `Empty`.
- Runners `local` e `bwrap` com caminhos fixos `/workspace` e `/claude` e o `.git` do repositório
  original montado no caminho original; adaptador Claude (monta comando, `--resume`,
  parse do `stream-json`, captura de usage).
- Testes do adaptador com um binário `claude` falso que emite `stream-json` roteirizado.
- **Pronto quando:** um job executa o agente (falso nos testes, real no smoke) num worktree e grava usage.
- Decidido: o login roda `claude setup-token` num diretório de config temporário e pede para colar
  o token (ou uma API key); o CLI do Claude vem do mise (`npm:@anthropic-ai/claude-code`, com
  `postinstall` que instala o binário nativo); sem retry automático — job com lease vencido volta
  à fila uma vez (`max_attempts = 2`, a engine da F6 enfileira assim) e falha do agente vai para a
  saída `failed`; a F5 é provada pelos testes (o primeiro disparo real vem com a F6).
- Detalhes: os dois providers (`LocalWorktree` e `Empty`) ficaram num só `workspace.Local`;
  o Claude roda com `--permission-mode acceptEdits` e sempre com `git commit`/`git push`
  bloqueados; a saída estruturada (MCP `submit_output` ou `--json-schema` do CLI) fica para a F7.

### F6 — Engine de fluxo
- Migrations `flow_runs`; triggers `card_created` / `card_entered_lane`.
- Execução de Agent, MoveCard, End; roteamento por saída; regra de reentrada com `--resume`.
- Eventos na linha do tempo do card.
- **Pronto quando:** card entra na baia → agente roda → card é movido pelo fluxo.
- Decidido: um run ativo por card + fluxo (disparo repetido é ignorado e registrado na linha do
  tempo; fluxos diferentes podem rodar juntos); card com run ativo não pode ser movido à mão
  (`409 card_busy`, arraste desabilitado); selo do último run no card e linha do tempo simples no
  modal (o painel completo é a F8); provado por teste de ponta a ponta com o `claude` falso.
- Detalhes: criar um card numa baia conta como entrar nela; saída sem ligação encerra o run
  (`failed` se o nó falhou); o nó Review falha o run até a F7; jobs de agente ainda sem
  `requirements` de host (MVP de host único); `node_runs.continued_at` evita seguir duas vezes a
  saída de um job repetido.

### F7 — MCP, perguntas e review
- Servidor MCP (HTTP streamable) com tools de integração e tools de agente
  (`ask_human`, `submit_output`, `submit_review`, `report_done`, `get_card`), token por NodeRun.
- Validação de `submit_output` contra o schema do nó; saída disponível aos nós seguintes.
- Migrations `questions`, `review_comments`; estado `awaiting_input` e retomada com respostas.
- Nó Review com `approved` / `changes_requested` / `max_rounds_exceeded`.
- **Pronto quando:** ciclo dev → review → comentários → retomada → aprovado funciona.

### F8 — Painel do card
- Linha do tempo, saída do agente ao vivo, perguntas com resposta, comentários de review,
  consumo agregado (tokens, turnos, custo estimado).
- **Pronto quando:** tudo do card visível e operável pela UI.

### F9 — Smoke de ponta a ponta
- Roteiro manual com Claude real: criar fluxo Dev → Review → MoveCard, criar card num repo de
  exemplo, responder uma pergunta, passar por um ciclo de review.
- Ajustes e documentação de uso (`README`).

## Fase 2 — Orquestração entre cards

### F10 — Approval
- Nó `Approval` (`approved` / `rejected` com comentário); tela de aprovação no painel do card
  exibindo a saída estruturada do nó anterior; rejeição pode voltar ao Agent (reentrada).

### F11 — CreateCards e WaitChildren
- Nó `CreateCards`: um card filho por item da saída estruturada (tipo, baia, projeto por nome).
- Nó `WaitChildren`: engine acorda o fluxo do pai a cada evento de filho; saídas `all_done` / `some_failed`.
- UI: filhos listados no card pai, progresso agregado.

### F12 — Smoke do cenário de referência
- Fluxo "Proposta" (Sourcegraph → Approval → CreateCards → WaitChildren) e fluxo
  "Implementação" (Agent ⇄ Review) de ponta a ponta com Claude real.

## Fora do MVP e da Fase 2
Nó de PR, review entre repositórios no fluxo do pai (`WaitSignal`), Fork/Join, projetos com origem
`git` (`GitClone`), storage compartilhado entre hosts, multiusuário/auth, outros agentes, webhooks,
orçamento de custo, runner Docker,
CI remoto (gates rodam localmente via mise tasks até existir um remote).
