-- name: InsertBattle :one
INSERT INTO battles (world_id, player_id, city_id, province_id, state, sent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBattle :one
SELECT * FROM battles WHERE id = $1;

-- name: GetBattleForUpdate :one
SELECT * FROM battles WHERE id = $1 FOR UPDATE;

-- name: GetActiveBattle :one
SELECT * FROM battles WHERE city_id = $1 AND status = 'active' LIMIT 1;

-- name: UpdateBattleState :exec
UPDATE battles SET state = $2, status = $3 WHERE id = $1;
