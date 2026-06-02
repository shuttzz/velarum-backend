-- name: CreatePlayer :one
INSERT INTO players (world_id, account_id, username, faction)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetPlayer :one
SELECT * FROM players WHERE id = $1;

-- name: GetPlayerByAccountAndWorld :one
SELECT * FROM players WHERE world_id = $1 AND account_id = $2;
