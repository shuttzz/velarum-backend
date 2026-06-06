-- +goose Up
-- ESPIONAGEM (SW3). Migration ADITIVA (1ª do fluxo append-only — não editar a 00001).
-- Batedores: unidade NÃO-combatente, treinada na Toca dos Batedores (contagem própria), enviada
-- em missões de scout numa LANE SEPARADA da militar. Revela intel do alvo (guarnição/defesa/saqueável).

-- Contagem de batedores TREINADOS, parados na cidade (separada da guarnição militar).
ALTER TABLE cities ADD COLUMN scouts INTEGER NOT NULL DEFAULT 0;

-- Fila de TREINO de batedores (lote conclui em finish_at; espelhada em scheduled_events scout.complete).
CREATE TABLE scout_queue (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    city_id    UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    count      INTEGER NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finish_at  TIMESTAMPTZ NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending', -- pending | completed | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_scout_queue_pending ON scout_queue (finish_at) WHERE status = 'pending';

-- Missões de espionagem: 1 batedor vai à cidade alvo, observa e volta com intel. Lane SEPARADA
-- (não conta na fila de marcha militar). Espelhada em scheduled_events (scout.arrival/return).
CREATE TABLE scout_missions (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    world_id         UUID NOT NULL REFERENCES worlds(id),
    attacker_city_id UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    target_city_id   UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'outbound', -- outbound | returning | done
    intel            JSONB,                            -- snapshot revelado (guarnição/defesa/saqueável)
    depart_at        TIMESTAMPTZ NOT NULL,
    arrive_at        TIMESTAMPTZ NOT NULL,
    return_at        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_scout_missions_active ON scout_missions (attacker_city_id) WHERE status <> 'done';

-- +goose Down
DROP TABLE IF EXISTS scout_missions;
DROP TABLE IF EXISTS scout_queue;
ALTER TABLE cities DROP COLUMN scouts;
