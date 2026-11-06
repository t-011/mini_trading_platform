-- +goose Up
CREATE TABLE trades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stock_id UUID NOT NULL REFERENCES stocks(id),
    type TEXT NOT NULL CHECK (type IN ('buy', 'sell')),                 
    qty NUMERIC(20, 8) NOT NULL CHECK (qty > 0),          -- allowing for fractional shares
    price BIGINT NOT NULL CHECK (price > 0),              -- cents
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE trades;