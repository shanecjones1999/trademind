CREATE TABLE IF NOT EXISTS paper_positions (
    account_id TEXT NOT NULL REFERENCES paper_accounts (id) ON DELETE CASCADE,
    symbol TEXT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity >= 0),
    cost_basis_cents BIGINT NOT NULL CHECK (cost_basis_cents >= 0),
    realized_pnl_cents BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, symbol)
);

CREATE INDEX IF NOT EXISTS paper_positions_account_symbol_idx
    ON paper_positions (account_id, symbol);
