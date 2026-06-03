-- name: CountPlayerProvinces :one
SELECT count(*) FROM provinces WHERE player_id = $1;

-- name: InsertProvince :one
INSERT INTO provinces (world_id, player_id, name_key, q, r, ring, def_attack, def_hp, reward_matter, reward_energy, reward_knowledge)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: ListPlayerProvinces :many
SELECT * FROM provinces WHERE player_id = $1 ORDER BY ring, q, r;

-- name: GetProvinceForUpdate :one
SELECT * FROM provinces WHERE id = $1 FOR UPDATE;

-- name: SetProvinceConquered :exec
UPDATE provinces SET status = 'conquered', conquered_at = $2 WHERE id = $1;
