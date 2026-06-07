-- name: InsertAlliance :one
INSERT INTO alliances (world_id, name, tag, owner_player_id, entry_mode)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAlliance :one
SELECT * FROM alliances WHERE id = $1;

-- name: GetAllianceForUpdate :one
SELECT * FROM alliances WHERE id = $1 FOR UPDATE;

-- name: ListAlliances :many
SELECT a.*, count(m.player_id) AS members
FROM alliances a LEFT JOIN alliance_members m ON m.alliance_id = a.id
WHERE a.world_id = $1
GROUP BY a.id
ORDER BY members DESC, a.created_at;

-- name: UpdateAllianceEntryMode :exec
UPDATE alliances SET entry_mode = $2 WHERE id = $1;

-- name: DeleteAlliance :exec
DELETE FROM alliances WHERE id = $1;

-- name: InsertAllianceMember :exec
INSERT INTO alliance_members (alliance_id, player_id, role) VALUES ($1, $2, $3);

-- name: GetMembershipByPlayer :one
SELECT * FROM alliance_members WHERE player_id = $1;

-- name: GetAllianceMemberForUpdate :one
SELECT * FROM alliance_members WHERE alliance_id = $1 AND player_id = $2 FOR UPDATE;

-- name: ListAllianceMembers :many
SELECT m.player_id, m.role, m.joined_at, p.username
FROM alliance_members m JOIN players p ON p.id = m.player_id
WHERE m.alliance_id = $1
ORDER BY m.joined_at;

-- name: CountAllianceMembers :one
SELECT count(*) FROM alliance_members WHERE alliance_id = $1;

-- name: UpdateMemberRole :exec
UPDATE alliance_members SET role = $3 WHERE alliance_id = $1 AND player_id = $2;

-- name: DeleteAllianceMember :exec
DELETE FROM alliance_members WHERE alliance_id = $1 AND player_id = $2;

-- name: InsertJoinRequest :exec
INSERT INTO alliance_join_requests (alliance_id, player_id) VALUES ($1, $2)
ON CONFLICT (alliance_id, player_id) DO NOTHING;

-- name: ListJoinRequests :many
SELECT r.id, r.player_id, r.created_at, p.username
FROM alliance_join_requests r JOIN players p ON p.id = r.player_id
WHERE r.alliance_id = $1
ORDER BY r.created_at;

-- name: GetJoinRequest :one
SELECT * FROM alliance_join_requests WHERE id = $1;

-- name: DeleteJoinRequest :exec
DELETE FROM alliance_join_requests WHERE id = $1;

-- name: DeleteJoinRequestsByPlayer :exec
DELETE FROM alliance_join_requests WHERE player_id = $1;
