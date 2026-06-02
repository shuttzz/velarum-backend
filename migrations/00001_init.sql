-- +goose Up
-- IDs em UUID v7 (uuidv7() é nativo no PostgreSQL 18): ordenados no tempo,
-- com melhor localidade de índice/inserts que o v4 aleatório.

-- Mundos (cada temporada é um mundo). world_id aparece em todas as tabelas (sharding por mundo).
CREATE TABLE worlds (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    name       TEXT NOT NULL,
    speed      SMALLINT NOT NULL DEFAULT 1,
    status     TEXT NOT NULL DEFAULT 'active', -- active | ended
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE players (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id      UUID NOT NULL REFERENCES worlds(id),
    username      TEXT NOT NULL,
    email         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    faction       TEXT NOT NULL, -- aurenthos | brevali | sorenthai | kethari | valdruun
    era           SMALLINT NOT NULL DEFAULT 1,
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, username),
    UNIQUE (world_id, email)
);

-- Cidade única do jogador. Estado de recursos por LAZY EVALUATION:
-- guarda o snapshot (*_stored), as taxas/h (*_rate), os tetos (*_cap) e o instante do último cálculo.
CREATE TABLE cities (
    id                   UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id             UUID NOT NULL REFERENCES worlds(id),
    player_id            UUID NOT NULL REFERENCES players(id),
    name                 TEXT NOT NULL,
    coord_x              INTEGER NOT NULL,
    coord_y              INTEGER NOT NULL,
    era                  SMALLINT NOT NULL DEFAULT 1,
    matter_stored        DOUBLE PRECISION NOT NULL DEFAULT 0,
    energy_stored        DOUBLE PRECISION NOT NULL DEFAULT 0,
    knowledge_stored     DOUBLE PRECISION NOT NULL DEFAULT 0,
    matter_rate          DOUBLE PRECISION NOT NULL DEFAULT 0, -- por hora
    energy_rate          DOUBLE PRECISION NOT NULL DEFAULT 0,
    knowledge_rate       DOUBLE PRECISION NOT NULL DEFAULT 0,
    matter_cap           DOUBLE PRECISION NOT NULL DEFAULT 500,
    energy_cap           DOUBLE PRECISION NOT NULL DEFAULT 500,
    knowledge_cap        DOUBLE PRECISION NOT NULL DEFAULT 200,
    resources_updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, coord_x, coord_y)
);
CREATE INDEX idx_cities_player ON cities (player_id);

-- Edifícios na GRADE da cidade: id estável (identidade) + posição (x,y) móvel.
CREATE TABLE city_buildings (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    city_id       UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    building_type TEXT NOT NULL,
    level         SMALLINT NOT NULL DEFAULT 1,
    pos_x         INTEGER NOT NULL,
    pos_y         INTEGER NOT NULL
);
CREATE INDEX idx_city_buildings_city ON city_buildings (city_id);

-- Fila de construção. finish_at é a verdade temporal (espelhada em scheduled_events).
CREATE TABLE build_queue (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    city_id       UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    building_id   UUID,                            -- NULL = construção nova; preenchido = upgrade
    building_type TEXT NOT NULL,
    target_level  SMALLINT NOT NULL,
    pos_x         INTEGER NOT NULL,
    pos_y         INTEGER NOT NULL,
    started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    finish_at     TIMESTAMPTZ NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending', -- pending | completed | cancelled
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_build_queue_pending ON build_queue (finish_at) WHERE status = 'pending';

-- Fonte de verdade dos eventos futuros agendados (scheduler).
-- Recarregada no boot -> eventos sobrevivem a restart. Processamento idempotente.
CREATE TABLE scheduled_events (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id   UUID REFERENCES worlds(id),
    type       TEXT NOT NULL, -- build.complete | troop.arrival | ...
    fires_at   TIMESTAMPTZ NOT NULL,
    payload    JSONB NOT NULL DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'pending', -- pending | processed
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_scheduled_events_due ON scheduled_events (fires_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS scheduled_events;
DROP TABLE IF EXISTS build_queue;
DROP TABLE IF EXISTS city_buildings;
DROP TABLE IF EXISTS cities;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS worlds;
