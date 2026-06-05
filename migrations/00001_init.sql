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

-- Conta GLOBAL do jogador (identidade que persiste entre mundos/temporadas).
-- Login é por email; username é o nome público. Unicidade case-insensitive via índices.
CREATE TABLE accounts (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    username      TEXT NOT NULL,
    email         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_accounts_email ON accounts (lower(email));
CREATE UNIQUE INDEX uq_accounts_username ON accounts (lower(username));

-- Sessão server-side. O cookie httpOnly carrega só um token opaco; aqui guardamos o
-- seu hash (sha256). Permite logout/revogação e "sair de todos os dispositivos".
CREATE TABLE sessions (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_account ON sessions (account_id);

-- Player: o "avatar" da conta DENTRO de um mundo. Uma conta tem no máximo um player por mundo.
CREATE TABLE players (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id     UUID NOT NULL REFERENCES worlds(id),
    account_id   UUID NOT NULL REFERENCES accounts(id),
    username     TEXT NOT NULL,
    faction      TEXT NOT NULL, -- aurenthos | brevali | sorenthai | kethari | valdruun
    era          SMALLINT NOT NULL DEFAULT 1,
    last_seen_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, account_id),
    UNIQUE (world_id, username)
);

-- Cidade única do jogador. Estado de recursos por LAZY EVALUATION:
-- guarda o snapshot (*_stored), as taxas/h (*_rate), os tetos (*_cap) e o instante do último cálculo.
CREATE TABLE cities (
    id                   UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id             UUID NOT NULL REFERENCES worlds(id),
    player_id            UUID NOT NULL REFERENCES players(id),
    name                 TEXT NOT NULL,
    region               TEXT NOT NULL DEFAULT '', -- região nomeada do mundo onde a cidade nasceu
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

-- Guarnição de tropas da cidade: quantidade por tipo de unidade.
CREATE TABLE city_troops (
    id        UUID PRIMARY KEY DEFAULT uuidv7(),
    city_id   UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    unit_type TEXT NOT NULL,
    count     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (city_id, unit_type)
);
CREATE INDEX idx_city_troops_city ON city_troops (city_id);

-- Fila de recrutamento. O lote inteiro conclui em finish_at (espelhado em scheduled_events).
CREATE TABLE recruit_queue (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    city_id    UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    unit_type  TEXT NOT NULL,
    count      INTEGER NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finish_at  TIMESTAMPTZ NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending', -- pending | completed | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_recruit_queue_pending ON recruit_queue (finish_at) WHERE status = 'pending';

-- Províncias PvE — INSTANCIADAS por jogador (cada um tem o seu conjunto a conquistar).
-- Mapa hex com a cidade no centro (0,0); anel 1 = Era 1. Coords axiais (q,r).
CREATE TABLE provinces (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id         UUID NOT NULL REFERENCES worlds(id),
    player_id        UUID NOT NULL REFERENCES players(id),
    name_key         TEXT NOT NULL,
    q                INTEGER NOT NULL,
    r                INTEGER NOT NULL,
    ring             SMALLINT NOT NULL,
    def_attack       INTEGER NOT NULL,
    def_hp           INTEGER NOT NULL,
    reward_matter    DOUBLE PRECISION NOT NULL DEFAULT 0,
    reward_energy    DOUBLE PRECISION NOT NULL DEFAULT 0,
    reward_knowledge DOUBLE PRECISION NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'unconquered', -- unconquered | conquered
    conquered_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (player_id, q, r)
);
CREATE INDEX idx_provinces_player ON provinces (player_id);

-- Marchas: exército que saiu da cidade rumo a uma província. Marcha = TIMER (ida → combate
-- → volta), espelhado em scheduled_events (troop.arrival / troop.return).
CREATE TABLE marches (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id     UUID NOT NULL REFERENCES worlds(id),
    city_id      UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    province_id  UUID NOT NULL REFERENCES provinces(id),
    troops       JSONB NOT NULL,            -- {unit_type: count} enviado
    survivors    JSONB,                     -- {unit_type: count} após o combate
    attacker_won BOOLEAN,
    status       TEXT NOT NULL DEFAULT 'outbound', -- outbound | returning | done
    depart_at    TIMESTAMPTZ NOT NULL,
    arrive_at    TIMESTAMPTZ NOT NULL,      -- chegada ao destino (combate)
    return_at    TIMESTAMPTZ,               -- chegada de volta à cidade
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_marches_active ON marches (city_id) WHERE status <> 'done';

-- Batalhas táticas (instanciadas): estado completo serializado em JSONB (servidor autoritativo,
-- determinístico). O jogador joga turnos; o defensor é IA. Cf. GDD §9 e internal/domain/battle.
CREATE TABLE battles (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id    UUID NOT NULL REFERENCES worlds(id),
    player_id   UUID NOT NULL REFERENCES players(id),
    city_id     UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    province_id UUID NOT NULL REFERENCES provinces(id),
    state       JSONB NOT NULL,
    sent        JSONB NOT NULL DEFAULT '{}', -- {unit_type: count} enviado (p/ o relatório)
    status      TEXT NOT NULL DEFAULT 'active', -- active | resolved
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_battles_active ON battles (city_id) WHERE status = 'active';

-- Relatórios/notificações do jogador (caixa de entrada): hoje, relatórios de batalha.
-- payload JSONB guarda os detalhes específicos por `type`.
CREATE TABLE reports (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id   UUID NOT NULL REFERENCES worlds(id),
    player_id  UUID NOT NULL REFERENCES players(id),
    type       TEXT NOT NULL, -- battle | ...
    payload    JSONB NOT NULL DEFAULT '{}',
    read       BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reports_player ON reports (player_id, created_at DESC);

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

-- Alvos PvE do MUNDO COMPARTILHADO (tiles que todos veem). SW2: 'node' = nó de recurso
-- (coleta ao longo do tempo, estilo RoK). Futuro: 'village'/'creature' (combate one-shot).
-- 1 ocupante por vez (occupied_by aponta a world_march que está coletando). Coords são do mundo.
CREATE TABLE world_targets (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id         UUID NOT NULL REFERENCES worlds(id),
    kind             TEXT NOT NULL,                       -- node | village | creature
    resource         TEXT NOT NULL,                       -- matter | energy | knowledge (o que o nó rende)
    level            INTEGER NOT NULL,
    coord_x          INTEGER NOT NULL,
    coord_y          INTEGER NOT NULL,
    amount_total     DOUBLE PRECISION NOT NULL,           -- nó: capacidade no spawn (0 p/ combate)
    amount_remaining DOUBLE PRECISION NOT NULL,           -- nó: restante (depleção PARCIAL)
    def_attack       INTEGER NOT NULL DEFAULT 0,          -- combate (village/creature): ataque agregado da defesa
    def_hp           INTEGER NOT NULL DEFAULT 0,          -- combate: HP agregado da defesa
    reward_matter    DOUBLE PRECISION NOT NULL DEFAULT 0, -- combate: loot ao matar
    reward_energy    DOUBLE PRECISION NOT NULL DEFAULT 0,
    reward_knowledge DOUBLE PRECISION NOT NULL DEFAULT 0,
    status           TEXT NOT NULL DEFAULT 'idle',        -- node: idle|occupied|depleted · combate: alive('idle')|depleted(morto)
    occupied_by      UUID,                                -- nó: world_march coletando agora (lock de 1 ocupante)
    expires_at       TIMESTAMPTZ,                         -- combate: TTL — despawna se ninguém atacar (NULL p/ nó)
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (world_id, coord_x, coord_y)
);
CREATE INDEX idx_world_targets_world ON world_targets (world_id) WHERE status <> 'depleted';

-- Marchas a alvos do mundo compartilhado (SW2). Distintas de `marches` (províncias privadas):
-- ida → coleta (timer ∝ carga÷taxa) → volta com loot. Espelhadas em scheduled_events
-- (world.arrival / world.collect / world.return).
CREATE TABLE world_marches (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id      UUID NOT NULL REFERENCES worlds(id),
    city_id       UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    target_id     UUID NOT NULL REFERENCES world_targets(id),
    troops        JSONB NOT NULL,                         -- {unit_type: count} enviado
    survivors     JSONB,                                  -- {unit_type: count} que volta (= enviado em nó; pós-combate em raid)
    loot          JSONB,                                  -- recurso coletado/saqueado (resource.Amounts; vazio se bounce/derrota)
    attacker_won  BOOLEAN,                                -- raid (village/creature): venceu? NULL = marcha de coleta (nó)
    status        TEXT NOT NULL DEFAULT 'outbound',       -- outbound | collecting | returning | done
    depart_at     TIMESTAMPTZ NOT NULL,
    arrive_at     TIMESTAMPTZ NOT NULL,                   -- chegada ao nó
    collect_until TIMESTAMPTZ,                            -- fim da coleta
    return_at     TIMESTAMPTZ,                            -- chegada de volta à cidade
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_world_marches_active ON world_marches (city_id) WHERE status <> 'done';
CREATE INDEX idx_world_marches_target ON world_marches (target_id) WHERE status <> 'done';

-- Mundo padrão (compartilhado) com UUID FIXO conhecido — os jogadores entram nele ao se
-- registrarem/logarem. Mais mundos/temporadas virão depois (matchmaking dedicado).
INSERT INTO worlds (id, name, speed, status)
VALUES ('00000000-0000-7000-8000-000000000001', 'Velarum', 1, 'active');

-- +goose Down
DROP TABLE IF EXISTS world_marches;
DROP TABLE IF EXISTS world_targets;
DROP TABLE IF EXISTS scheduled_events;
DROP TABLE IF EXISTS battles;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS marches;
DROP TABLE IF EXISTS provinces;
DROP TABLE IF EXISTS recruit_queue;
DROP TABLE IF EXISTS city_troops;
DROP TABLE IF EXISTS build_queue;
DROP TABLE IF EXISTS city_buildings;
DROP TABLE IF EXISTS cities;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS worlds;
