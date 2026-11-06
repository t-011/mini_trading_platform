-- name: GetBarsByInterval :many
SELECT date_trunc(sqlc.arg(interval), timestamp) AS bucket,          -- 'day','week','month'...
        (array_agg(open  ORDER BY timestamp ASC ))[1] AS open,
        max(high) AS high, min(low) AS low,
        (array_agg(close ORDER BY timestamp DESC))[1] AS close,
        sum(volume) AS volume
    FROM bars_1min
    WHERE stock_id = sqlc.arg(stock_id) 
        AND timestamp >= sqlc.arg(timestamp_from) AND timestamp < sqlc.arg(timestamp_to)
    GROUP BY bucket
    ORDER BY bucket;

-- name: GetLatestClose :one
SELECT close FROM bars_1min
    WHERE stock_id = $1
    ORDER BY timestamp DESC
LIMIT 1;