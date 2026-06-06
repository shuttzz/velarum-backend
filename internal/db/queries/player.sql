-- name: CreatePlayer :one
INSERT INTO players (world_id, account_id, username, faction, shield_until)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPlayer :one
SELECT * FROM players WHERE id = $1;

-- name: GetPlayerByAccountAndWorld :one
SELECT * FROM players WHERE world_id = $1 AND account_id = $2;

-- name: GetPlayerForUpdate :one
SELECT * FROM players WHERE id = $1 FOR UPDATE;

-- name: DropPlayerShield :exec
-- Derruba o escudo de novato (ao fazer o 1º ataque).
UPDATE players SET shield_until = NULL WHERE id = $1;
