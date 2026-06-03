-- name: InsertReport :one
INSERT INTO reports (world_id, player_id, type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPlayerReports :many
SELECT * FROM reports WHERE player_id = $1 ORDER BY created_at DESC LIMIT 50;

-- name: MarkAllReportsRead :exec
UPDATE reports SET read = true WHERE player_id = $1 AND read = false;
