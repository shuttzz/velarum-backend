-- name: InsertWorldMarch :one
INSERT INTO world_marches (world_id, city_id, target_id, troops, status, depart_at, arrive_at)
VALUES ($1, $2, $3, $4, 'outbound', $5, $6)
RETURNING *;

-- name: GetWorldMarchForUpdate :one
SELECT * FROM world_marches WHERE id = $1 FOR UPDATE;

-- name: ListActiveWorldMarches :many
SELECT * FROM world_marches WHERE city_id = $1 AND status <> 'done' ORDER BY arrive_at;

-- name: SetWorldMarchCollecting :exec
UPDATE world_marches SET status = 'collecting', loot = $2, collect_until = $3 WHERE id = $1;

-- name: SetWorldMarchReturning :exec
UPDATE world_marches SET status = 'returning', survivors = $2, return_at = $3 WHERE id = $1;

-- name: SetWorldMarchCombatReturning :exec
-- Raid (village/creature): após o combate no destino, volta com sobreviventes + loot + resultado.
UPDATE world_marches SET status = 'returning', survivors = $2, loot = $3, attacker_won = $4, return_at = $5 WHERE id = $1;

-- name: SetWorldMarchDone :exec
UPDATE world_marches SET status = 'done' WHERE id = $1;
