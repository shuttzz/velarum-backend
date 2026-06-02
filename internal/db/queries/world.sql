-- name: CreateWorld :one
INSERT INTO worlds (name, speed)
VALUES ($1, $2)
RETURNING *;

-- name: GetWorld :one
SELECT * FROM worlds WHERE id = $1;
