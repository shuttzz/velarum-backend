-- name: InsertBuildQueue :one
INSERT INTO build_queue (city_id, building_id, building_type, target_level, pos_x, pos_y, started_at, finish_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetBuildQueueForUpdate :one
SELECT * FROM build_queue WHERE id = $1 FOR UPDATE;

-- name: CompleteBuildQueue :execrows
UPDATE build_queue SET status = 'completed'
WHERE id = $1 AND status = 'pending';

-- name: ListPendingBuilds :many
SELECT id, building_id, building_type, target_level, pos_x, pos_y, finish_at
FROM build_queue
WHERE city_id = $1 AND status = 'pending'
ORDER BY finish_at;
