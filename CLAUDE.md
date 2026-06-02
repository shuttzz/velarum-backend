# Backend (Go) — Velarum

API e lógica de jogo. Design completo do jogo: `docs/GDD.md`. Contexto global: `~/.claude/CLAUDE.md`.

## Stack
Go (module `backend`) · chi (HTTP) · pgx/v5 + sqlc · goose (migrations) · PostgreSQL 18 · Redis.

## Arquitetura
- Monolito modular. Lógica de domínio PURA, sem I/O, em `internal/domain/*` (testável isolada).
- Lazy evaluation de recursos (`internal/domain/resource`).
- Eventos futuros agendados, persistidos e recarregados no boot (`internal/scheduler`).
- App stateless. `world_id` em toda tabela (sharding por mundo). PostgreSQL em `internal/pg`.

## Convenções
- IDs: **sempre UUID v7** (`uuidv7()` nativo do PG18). Nunca v4 / `gen_random_uuid()`.
- **Todo código com testes unitários.** Domínio puro sem I/O.
- Integração de DB: testes contra Postgres LOCAL (compose), gated por `TEST_DATABASE_URL`. Nunca nuvem.
- Ao final de cada etapa concluída: rodar **/publish** (commit + versão SemVer + push).

## Rodar / testar (Docker — sem Go local)
- `make up` → API (:8080) + Postgres (host :5433) + Redis
- `make test` → testes unitários
- `make itest` → testes de integração (sobe o Postgres do compose)
