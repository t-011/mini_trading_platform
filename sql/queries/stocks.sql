
-- name: GetStockBySymbolOrName :many
SELECT * FROM stocks 
    WHERE symbol ILIKE '%' || sqlc.arg(search) || '%' OR name ILIKE '%' || sqlc.arg(search) || '%'
    ORDER BY symbol
    LIMIT $1 OFFSET $2;

-- name: GetStockById :one
SELECT * FROM stocks
    WHERE id = $1;
    