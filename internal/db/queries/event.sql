-- name: InsertScheduledEvent :one
INSERT INTO scheduled_events (type, fires_at, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DuePendingEvents :many
SELECT * FROM scheduled_events
WHERE status = 'pending' AND fires_at <= $1
ORDER BY fires_at;

-- name: MarkEventProcessed :exec
UPDATE scheduled_events SET status = 'processed' WHERE id = $1;
