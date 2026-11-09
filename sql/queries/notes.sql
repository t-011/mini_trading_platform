-- name: ListNotesByTrade :many
SELECT n.* FROM trade_notes n
    JOIN trades t ON t.id = n.trade_id
    WHERE n.trade_id = $1 AND t.user_id = $2
    ORDER BY n.created_at;

-- name: CreateNote :one
INSERT INTO trade_notes (trade_id, body)
    SELECT sqlc.arg(trade_id), sqlc.arg(body)
    WHERE EXISTS (
        SELECT 1 FROM trades WHERE id = sqlc.arg(trade_id) AND user_id = sqlc.arg(user_id)
    )
    RETURNING *;
