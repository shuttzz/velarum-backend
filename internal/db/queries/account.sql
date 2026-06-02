-- name: CreateAccount :one
INSERT INTO accounts (username, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetAccountByID :one
SELECT * FROM accounts WHERE id = $1;

-- name: GetAccountByEmail :one
SELECT * FROM accounts WHERE lower(email) = lower($1);

-- name: TouchAccountLogin :exec
UPDATE accounts SET last_login_at = $2 WHERE id = $1;
