-- +goose Up
-- Limit orders sent to our own exchange (engine/). The BIGSERIAL id is also
-- the engine's u64 order_id. Quantities are whole shares here; the engine
-- protocol carries them as 1e-8 units.
CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stock_id UUID NOT NULL REFERENCES stocks(id),
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    price BIGINT NOT NULL CHECK (price > 0),                -- limit, cents
    qty BIGINT NOT NULL CHECK (qty > 0),                    -- whole shares
    filled BIGINT NOT NULL DEFAULT 0 CHECK (filled >= 0),
    canceled BIGINT NOT NULL DEFAULT 0 CHECK (canceled >= 0),
    status TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'filled', 'canceled', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (filled + canceled <= qty)
);

CREATE INDEX orders_open_by_user ON orders (user_id, stock_id) WHERE status = 'open';

-- Our exchange's one tradable stock.
INSERT INTO stocks (symbol, name, exchange, asset_type, currency)
    VALUES ('MINI', 'Mini Corp', 'MINI_EXCHANGE', 'stock', 'USD');

-- +goose Down
DROP TABLE orders;
DELETE FROM stocks WHERE symbol = 'MINI' AND exchange = 'MINI_EXCHANGE';
