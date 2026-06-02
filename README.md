# Velarum — Backend (Go)

Backend do jogo de estratégia de navegador (cidade única que evolui por eras). Design completo em [`docs/GDD.md`](docs/GDD.md).

> `velarum` é **nome de trabalho** (verificar marca antes do lançamento). O module Go é `backend` por ora — trocar para o path do repositório quando definido (`go mod edit -module <path>`).

## Princípios de desenvolvimento
- **Todo o backend é desenvolvido com testes unitários.** Cada pacote tem seu `_test.go`. A lógica de domínio é **pura** (sem I/O) — testável isoladamente, sem banco nem rede.
- Arquitetura: monolito modular; **lazy evaluation** de recursos; **eventos agendados** persistidos (sobrevivem a restart); app **stateless**; `world_id` em toda tabela (sharding por mundo).
- **Roda em Docker** — não é preciso instalar Go na máquina.

## Estrutura
```
cmd/server/                 # entrypoint (HTTP + scheduler)
internal/
  domain/resource/          # lógica PURA de recursos (lazy evaluation) + testes
  config/                   # constantes de jogo (tuning) — Era 1 preenchida + testes
  scheduler/                # scheduler de eventos futuros (Store; MemStore por ora) + testes
migrations/                 # migrations goose (schema PostgreSQL)
sqlc.yaml                   # config do sqlc (gerar acesso ao banco)
docker-compose.yml          # backend + Postgres + Redis (dev)
Dockerfile                  # build de produção (binário estático)
docs/GDD.md                 # game design document consolidado
```

## Rodar (via Docker — sem instalar Go)
```sh
docker compose up --build          # sobe backend + PostgreSQL + Redis
curl localhost:8080/health         # -> {"status":"ok","service":"velarum-backend"}
```

Rodar os testes unitários:
```sh
docker compose run --rm backend go test ./...
# ou: make test
```

## Próximos passos
1. Subir PostgreSQL/Redis (já no compose) e aplicar a migration `00001_init.sql` via goose.
2. Escrever queries em `internal/db/queries/` e gerar acesso com `sqlc generate`.
3. Implementar `PgStore` do scheduler (substituindo `MemStore`) sobre a tabela `scheduled_events`.
4. Primeiro caso de uso de cidade (com testes): criar cidade, enfileirar construção
   (gasto de recursos via `resource.Spend` em transação com lock), agendar `build.complete`.
