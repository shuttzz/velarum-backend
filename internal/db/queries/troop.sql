-- name: ListCityTroops :many
SELECT * FROM city_troops WHERE city_id = $1 ORDER BY unit_type;

-- name: AddCityTroops :exec
INSERT INTO city_troops (city_id, unit_type, count)
VALUES ($1, $2, $3)
ON CONFLICT (city_id, unit_type)
DO UPDATE SET count = city_troops.count + EXCLUDED.count;

-- name: InsertRecruitQueue :one
INSERT INTO recruit_queue (city_id, unit_type, count, started_at, finish_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPendingRecruits :many
SELECT id, unit_type, count, finish_at
FROM recruit_queue
WHERE city_id = $1 AND status = 'pending'
ORDER BY finish_at;

-- name: GetRecruitForUpdate :one
SELECT * FROM recruit_queue WHERE id = $1 FOR UPDATE;

-- name: CompleteRecruitQueue :execrows
UPDATE recruit_queue SET status = 'completed'
WHERE id = $1 AND status = 'pending';

-- name: CancelRecruitQueue :execrows
UPDATE recruit_queue SET status = 'cancelled'
WHERE id = $1 AND status = 'pending';
