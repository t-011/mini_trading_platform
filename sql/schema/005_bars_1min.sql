-- +goose Up
CREATE TABLE bars_1min(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stock_id UUID NOT NULL,
    open   BIGINT NOT NULL,   -- cents
    high   BIGINT NOT NULL,
    low    BIGINT NOT NULL,
    close  BIGINT NOT NULL,
    volume BIGINT NOT NULL,   -- share count (not money — just an int)
    timestamp TIMESTAMPTZ NOT NULL,
    FOREIGN KEY (stock_id) REFERENCES stocks(id) ON DELETE CASCADE,
    UNIQUE (stock_id, timestamp)
);

-- +goose Down
DROP TABLE bars_1min;