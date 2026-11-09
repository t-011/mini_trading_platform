-- name: CreateAccountForUserId :exec
INSERT INTO accounts (user_id)
    VALUES ($1);

-- name: Debit :execrows
UPDATE accounts
    SET cash = cash - sqlc.arg(cost)
    WHERE user_id = sqlc.arg(user_id) AND cash >= sqlc.arg(cost);

-- name: Credit :exec
UPDATE accounts
    SET cash = cash + sqlc.arg(cost)
    WHERE user_id = sqlc.arg(user_id);

-- name: GetCash :one
SELECT cash FROM accounts WHERE user_id = $1;

-- name: LockAccount :exec
-- Serializes a user's concurrent trades: taken at the top of the trade tx so
-- read-then-write sell checks can't race (two sells overselling a position).
SELECT id FROM accounts WHERE user_id = $1 FOR UPDATE;