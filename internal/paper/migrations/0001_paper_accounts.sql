CREATE TABLE IF NOT EXISTS paper_accounts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    opened_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS cash_ledger_entries (
    id BIGSERIAL PRIMARY KEY,
    transaction_id TEXT NOT NULL,
    entry_index SMALLINT NOT NULL,
    account_id TEXT NOT NULL,
    asset TEXT NOT NULL CHECK (asset = 'USD'),
    amount_cents BIGINT NOT NULL CHECK (amount_cents <> 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    description TEXT NOT NULL,
    UNIQUE (transaction_id, entry_index)
);

CREATE INDEX IF NOT EXISTS cash_ledger_entries_account_asset_idx
    ON cash_ledger_entries (account_id, asset);
