-- +goose Up
-- ALIANÇAS (SW3) — núcleo de membros. Migration aditiva (append-only). Cf. design-aliancas.
-- Papéis: owner | leader | officer | member. Entrada: open (entra direto) | approval (pedido).
-- 1 aliança por jogador (UNIQUE em alliance_members.player_id).

-- Moeda PREMIUM (nível de conta, global). Criar aliança custa premium (config.AllianceCreateCost).
-- Toda conta NOVA começa com 1000 (presente de onboarding — valor a balancear depois; é mecânica
-- de verdade, não só dev). Comprar mais = monetização (futuro, GDD §13).
ALTER TABLE accounts ADD COLUMN premium INTEGER NOT NULL DEFAULT 1000;

CREATE TABLE alliances (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id        UUID NOT NULL REFERENCES worlds(id),
    name            TEXT NOT NULL,
    tag             TEXT NOT NULL,
    owner_player_id UUID NOT NULL REFERENCES players(id),
    entry_mode      TEXT NOT NULL DEFAULT 'approval', -- open | approval
    member_cap      INTEGER NOT NULL DEFAULT 30,      -- cresce por pesquisa (futuro)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_alliances_name ON alliances (world_id, lower(name));
CREATE UNIQUE INDEX uq_alliances_tag ON alliances (world_id, lower(tag));

CREATE TABLE alliance_members (
    alliance_id UUID NOT NULL REFERENCES alliances(id) ON DELETE CASCADE,
    player_id   UUID NOT NULL REFERENCES players(id),
    role        TEXT NOT NULL DEFAULT 'member', -- owner | leader | officer | member
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (alliance_id, player_id),
    UNIQUE (player_id) -- 1 aliança por jogador (por mundo, pois player é por mundo)
);

CREATE TABLE alliance_join_requests (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    alliance_id UUID NOT NULL REFERENCES alliances(id) ON DELETE CASCADE,
    player_id   UUID NOT NULL REFERENCES players(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (alliance_id, player_id)
);
CREATE INDEX idx_alliance_requests_alliance ON alliance_join_requests (alliance_id);

-- +goose Down
DROP TABLE IF EXISTS alliance_join_requests;
DROP TABLE IF EXISTS alliance_members;
DROP TABLE IF EXISTS alliances;
