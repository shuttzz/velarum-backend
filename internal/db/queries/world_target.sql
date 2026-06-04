-- name: ListWorldTargets :many
SELECT * FROM world_targets WHERE world_id = $1 AND status <> 'depleted' ORDER BY id;

-- name: CountWorldTargets :one
SELECT count(*) FROM world_targets WHERE world_id = $1;

-- name: InsertWorldTarget :one
INSERT INTO world_targets (world_id, kind, resource, level, coord_x, coord_y, amount_total, amount_remaining)
VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
RETURNING *;

-- name: GetWorldTargetForUpdate :one
SELECT * FROM world_targets WHERE id = $1 FOR UPDATE;

-- name: ReserveWorldTarget :exec
-- Reserva o nó para coleta: decrementa o restante e trava 1 ocupante (occupied_by = world_march).
UPDATE world_targets SET amount_remaining = $2, status = 'occupied', occupied_by = $3 WHERE id = $1;

-- name: ReleaseWorldTarget :exec
-- Libera a ocupação ao fim da coleta (nó ainda tem recurso).
UPDATE world_targets SET status = 'idle', occupied_by = NULL WHERE id = $1;

-- name: RespawnWorldTarget :exec
-- Nó zerou → respawna na MESMA linha em outro lugar (novas coords/nível/recurso/quantidade cheia).
UPDATE world_targets SET kind = $2, resource = $3, level = $4, coord_x = $5, coord_y = $6,
    amount_total = $7, amount_remaining = $7, status = 'idle', occupied_by = NULL WHERE id = $1;
