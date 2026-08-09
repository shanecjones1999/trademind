CREATE TABLE IF NOT EXISTS paper_position_lots (
    id BIGSERIAL PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES paper_accounts (id) ON DELETE CASCADE,
    symbol TEXT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    cost_per_share_cents BIGINT NOT NULL CHECK (cost_per_share_cents > 0),
    acquired_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS paper_position_lots_account_symbol_idx
    ON paper_position_lots (account_id, symbol, acquired_at, id);
