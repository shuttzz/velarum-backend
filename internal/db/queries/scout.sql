-- name: InsertScoutQueue :one
INSERT INTO scout_queue (city_id, count, started_at, finish_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListPendingScouts :many
SELECT id, count, finish_at FROM scout_queue WHERE city_id = $1 AND status = 'pending' ORDER BY finish_at;

-- name: GetScoutQueueForUpdate :one
SELECT * FROM scout_queue WHERE id = $1 FOR UPDATE;

-- name: CompleteScoutQueue :execrows
UPDATE scout_queue SET status = 'completed' WHERE id = $1 AND status = 'pending';

-- name: InsertScoutMission :one
INSERT INTO scout_missions (world_id, attacker_city_id, target_city_id, status, depart_at, arrive_at)
VALUES ($1, $2, $3, 'outbound', $4, $5)
RETURNING *;

-- name: GetScoutMissionForUpdate :one
SELECT * FROM scout_missions WHERE id = $1 FOR UPDATE;

-- name: ListActiveScoutMissions :many
SELECT * FROM scout_missions WHERE attacker_city_id = $1 AND status <> 'done' ORDER BY arrive_at;

-- name: SetScoutMissionReturning :exec
UPDATE scout_missions SET status = 'returning', intel = $2, return_at = $3 WHERE id = $1;

-- name: SetScoutMissionDone :exec
UPDATE scout_missions SET status = 'done' WHERE id = $1;
