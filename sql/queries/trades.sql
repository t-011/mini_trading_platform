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

-- name: ListTradesByUser :many
SELECT * FROM trades
    WHERE user_id = $1
    ORDER BY timestamp DESC
    LIMIT $2 OFFSET $3;

-- name: GetHoldings :many
SELECT
    s.id AS stock_id, s.symbol, s.name,
    SUM(CASE WHEN t.type='buy' THEN t.qty ELSE -t.qty END)::numeric AS qty,
    -- average buy price in cents (weighted average, not FIFO lots)
    (SUM(t.qty * t.price) FILTER (WHERE t.type='buy')
        / NULLIF(SUM(t.qty) FILTER (WHERE t.type='buy'), 0))::numeric AS avg_cost,
    -- latest bar close; our own exchange's stocks have no bars, so fall back
    -- to their last trade price (a holding implies at least one trade)
    COALESCE(lc.close, (
        SELECT t2.price FROM trades t2
        WHERE t2.stock_id = s.id ORDER BY t2.timestamp DESC LIMIT 1
    ))::bigint AS last_close
FROM trades t
JOIN stocks s ON s.id = t.stock_id
LEFT JOIN LATERAL (
    SELECT close FROM bars_1min b
    WHERE b.stock_id = t.stock_id ORDER BY timestamp DESC LIMIT 1
) lc ON true
WHERE t.user_id = $1
GROUP BY s.id, s.symbol, s.name, lc.close
HAVING SUM(CASE WHEN t.type='buy' THEN t.qty ELSE -t.qty END) > 0;