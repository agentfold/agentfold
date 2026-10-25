# Orquestrator

Board de gerenciamento de projetos em que fluxos desenhados visualmente disparam agentes de IA
(Claude Code) quando cards são criados ou entram em uma baia. Expõe API REST e servidor MCP.

- Arquitetura: [docs/arquitetura.md](docs/arquitetura.md)
- Plano do MVP: [docs/plano-mvp.md](docs/plano-mvp.md)

## Estrutura

```
backend/   Go (binário único: server, worker, migrate, agent)
web/       React + Vite + TypeScript
docs/      Arquitetura e plano
```

## Desenvolvimento

Toolchains e tarefas são gerenciadas pelo [mise](https://mise.jdx.dev):

```bash
mise install
mise run web:install
mise run dev:setup    # cria .env com a ORQ_SECRET_KEY (uma vez)
mise run db:up        # Postgres de desenvolvimento (docker compose)
mise run check        # lint + testes com gate de 100% de cobertura
```

Para subir a API em `http://127.0.0.1:8080/api` (contrato em [api/openapi.yaml](api/openapi.yaml)):

```bash
mise run db:up
mise run backend:serve   # aplica as migrations e sobe o server
```

Para rodar fluxos e agentes:

```bash
mise run agent:login -- --profile dev   # roda `claude setup-token` e guarda o token criptografado
mise run backend:worker                 # executa os fluxos e os agentes (isolados com bwrap)
```

Os fluxos rodam no worker: criar um card ou movê-lo para outra baia enfileira um job que dispara
os fluxos habilitados cujo gatilho combina (sempre a última versão publicada). Criar um card numa
baia também conta como entrar nela. Enquanto um fluxo roda para um card, só o fluxo move o card,
e o mesmo fluxo não é disparado de novo para ele. As mudanças feitas pelo worker chegam ao board
ao vivo pelo server (`LISTEN/NOTIFY`).

Os agentes falam com o board pelo servidor MCP em `http://127.0.0.1:8080/mcp` (o worker o encontra
por `ORQ_SERVER_URL`, padrão `http://127.0.0.1:8080`). Cada execução recebe um token próprio e as
tools `get_card`, `ask_human`, `report_done`, `submit_output` (nós com schema de saída) e
`submit_review` (nós Review). Perguntas do agente aparecem na página do card (`#/cards/<id>`, aberta
pelo clique no card) para resposta; o card fica parado até todas serem respondidas. A página
mostra também a execução ao vivo (nós, saída do agente, comentários de review), a linha do tempo
e o consumo, e permite cancelar o fluxo. Sem token, o mesmo endpoint expõe tools de integração
(`list_cards`, `create_card`, `move_card`, `list_questions`, `answer_questions`, `get_run`…).

O worker lê `ORQ_DATA_DIR` (padrão `~/.local/share/orquestrator`, onde ficam os worktrees dos
cards), `ORQ_CLAUDE_BIN` (padrão: o `claude` do PATH, fixado no `mise.toml`) e `ORQ_RUNNER`
(`bwrap`, padrão, ou `local` sem isolamento).

Em outro terminal, `mise run web:dev` sobe o board em `http://localhost:5173` (o Vite repassa
`/api` para o server).

O binário lê `ORQ_DATABASE_URL` e `ORQ_SECRET_KEY` (obrigatórias) e `ORQ_ADDR` (padrão `127.0.0.1:8080`; a API ainda
não tem autenticação, por isso escuta só em loopback). Eventos do board em tempo real:
`GET /api/events` (Server-Sent Events).

Os testes do store e do CLI sobem um Postgres efêmero com testcontainers, então precisam de Docker.
Após mudar `backend/migrations`, `backend/internal/store/queries` ou `api/openapi.yaml`, rode
`mise run generate` (o lint falha se o código gerado estiver desatualizado).

Outras tarefas: `mise tasks`.

Excluídos da cobertura: `cmd/*/main.go`, `web/src/main.tsx` e código gerado.
