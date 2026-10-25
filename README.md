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
mise run db:up        # Postgres de desenvolvimento (docker compose)
mise run check        # lint + testes com gate de 100% de cobertura
```

Para subir a API em `http://127.0.0.1:8080/api` (contrato em [api/openapi.yaml](api/openapi.yaml)):

```bash
mise run db:up
mise run backend:serve   # aplica as migrations e sobe o server
```

O binário lê `ORQ_DATABASE_URL` (obrigatória) e `ORQ_ADDR` (padrão `127.0.0.1:8080`; a API ainda
não tem autenticação, por isso escuta só em loopback). Eventos do board em tempo real:
`GET /api/events` (Server-Sent Events).

Os testes do store e do CLI sobem um Postgres efêmero com testcontainers, então precisam de Docker.
Após mudar `backend/migrations`, `backend/internal/store/queries` ou `api/openapi.yaml`, rode
`mise run generate` (o lint falha se o código gerado estiver desatualizado).

Outras tarefas: `mise tasks`.

Excluídos da cobertura: `cmd/*/main.go`, `web/src/main.tsx` e código gerado.
