-- +goose Up
CREATE TABLE stocks(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    exchange TEXT NOT NULL,
    asset_type TEXT NOT NULL CHECK (asset_type IN ('stock', 'etf')),
    currency TEXT NOT NULL CHECK (currency IN ('USD', 'GBP', 'AUD')), -- later enum
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT stock_sym UNIQUE (symbol, exchange)
);

-- +goose Down
DROP TABLE stocks;