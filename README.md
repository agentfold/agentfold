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

Os testes do store sobem um Postgres efêmero com testcontainers, então precisam de Docker.
Após mudar `backend/migrations` ou `backend/internal/store/queries`, rode `mise run generate`
(o lint falha se o código do sqlc estiver desatualizado).

Outras tarefas: `mise tasks`.

Excluídos da cobertura: `cmd/*/main.go`, `web/src/main.tsx` e código gerado.
