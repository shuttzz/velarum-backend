-- name: InsertRaid :one
INSERT INTO raids (world_id, attacker_city_id, defender_city_id, troops, status, depart_at, arrive_at)
VALUES ($1, $2, $3, $4, 'outbound', $5, $6)
RETURNING *;

-- name: GetRaidForUpdate :one
SELECT * FROM raids WHERE id = $1 FOR UPDATE;

-- name: ListAttackerRaids :many
-- Saques que SAÍRAM desta cidade (ataques em andamento).
SELECT * FROM raids WHERE attacker_city_id = $1 AND status <> 'done' ORDER BY arrive_at;

-- name: ListIncomingRaids :many
-- Saques VINDO para esta cidade (alerta de incoming — defesa ativa).
SELECT * FROM raids WHERE defender_city_id = $1 AND status = 'outbound' ORDER BY arrive_at;

-- name: SetRaidResult :exec
UPDATE raids SET status = 'returning', attacker_won = $2, survivors = $3, loot = $4, return_at = $5 WHERE id = $1;

-- name: SetRaidDone :exec
UPDATE raids SET status = 'done' WHERE id = $1;
