-- name: CreateTrade :one
INSERT INTO trades(
    user_id,
    stock_id,
    type,
    qty,
    price
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPosition :one
SELECT COALESCE(SUM(CASE WHEN type='buy' THEN qty ELSE -qty END), 0)::numeric
    FROM trades
    WHERE user_id = $1 AND stock_id = $2;