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

Para rodar agentes:

```bash
mise run agent:login -- --profile dev   # roda `claude setup-token` e guarda o token criptografado
mise run backend:worker                 # executa os jobs de agente (isolados com bwrap)
```

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
