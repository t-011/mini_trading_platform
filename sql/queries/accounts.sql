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