-- name: InsertMarch :one
INSERT INTO marches (world_id, city_id, province_id, troops, status, depart_at, arrive_at)
VALUES ($1, $2, $3, $4, 'outbound', $5, $6)
RETURNING *;

-- name: GetMarchForUpdate :one
SELECT * FROM marches WHERE id = $1 FOR UPDATE;

-- name: ListActiveMarches :many
SELECT * FROM marches WHERE city_id = $1 AND status <> 'done' ORDER BY arrive_at;

-- name: SetMarchResult :exec
UPDATE marches SET status = 'returning', attacker_won = $2, survivors = $3, return_at = $4 WHERE id = $1;

-- name: SetMarchDone :exec
UPDATE marches SET status = 'done' WHERE id = $1;
