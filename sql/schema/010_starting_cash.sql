-- +goose Up
-- Paper trading: every new account starts with $100,000 (cents).
ALTER TABLE accounts ALTER COLUMN cash SET DEFAULT 10000000;

-- +goose Down
ALTER TABLE accounts ALTER COLUMN cash SET DEFAULT 0;
