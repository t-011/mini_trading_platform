-- name: CreateOrder :one
INSERT INTO orders (user_id, stock_id, side, price, qty)
    VALUES ($1, $2, $3, $4, $5)
    RETURNING *;

-- name: GetOrder :one
SELECT * FROM orders WHERE id = $1;

-- name: ListOrdersByUser :many
SELECT * FROM orders
    WHERE user_id = $1
    ORDER BY id DESC
    LIMIT $2 OFFSET $3;

-- name: FillOrder :one
-- Right-hand columns are the pre-update values.
UPDATE orders
    SET filled = filled + sqlc.arg(shares),
        status = CASE WHEN filled + sqlc.arg(shares) + canceled < qty THEN status
                      WHEN canceled > 0 THEN 'canceled'
                      ELSE 'filled' END
    WHERE id = sqlc.arg(id)
    RETURNING *;

-- name: CancelOrder :one
UPDATE orders
    SET canceled = canceled + sqlc.arg(shares),
        status = CASE WHEN filled + canceled + sqlc.arg(shares) < qty THEN status
                      ELSE 'canceled' END
    WHERE id = sqlc.arg(id)
    RETURNING *;

-- name: RejectOrder :exec
UPDATE orders SET status = 'rejected' WHERE id = $1;

-- name: GetReservedShares :one
-- Shares already promised to the user's open sell orders.
SELECT COALESCE(SUM(qty - filled - canceled), 0)::bigint
    FROM orders
    WHERE user_id = $1 AND stock_id = $2 AND side = 'sell' AND status = 'open';
