-- name: InsertBuildQueue :one
INSERT INTO build_queue (city_id, slot_index, building_type, target_level, started_at, finish_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBuildQueueForUpdate :one
SELECT * FROM build_queue WHERE id = $1 FOR UPDATE;

-- name: CompleteBuildQueue :execrows
UPDATE build_queue SET status = 'completed'
WHERE id = $1 AND status = 'pending';
