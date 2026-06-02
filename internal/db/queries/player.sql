-- name: CreatePlayer :one
INSERT INTO players (world_id, username, email, password_hash, faction)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlayer :one
SELECT * FROM players WHERE id = $1;
