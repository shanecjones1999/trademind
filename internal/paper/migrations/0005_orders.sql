CREATE TABLE IF NOT EXISTS paper_orders (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES paper_accounts (id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    symbol TEXT NOT NULL,
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    order_type TEXT NOT NULL CHECK (order_type IN ('market', 'limit')),
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    limit_price_cents BIGINT CHECK (limit_price_cents IS NULL OR limit_price_cents > 0),
    status TEXT NOT NULL CHECK (status IN ('open', 'filled')),
    submitted_at TIMESTAMPTZ NOT NULL,
    UNIQUE (account_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS paper_orders_account_submitted_idx
    ON paper_orders (account_id, submitted_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS paper_executions (
    id TEXT PRIMARY KEY,
    order_id TEXT NOT NULL UNIQUE REFERENCES paper_orders (id) ON DELETE CASCADE,
    symbol TEXT NOT NULL,
    quantity BIGINT NOT NULL CHECK (quantity > 0),
    price_cents BIGINT NOT NULL CHECK (price_cents > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    quote_as_of TIMESTAMPTZ NOT NULL,
    quote_source TEXT NOT NULL,
    gross_cents BIGINT NOT NULL CHECK (gross_cents <> 0),
    realized_pnl_cents BIGINT,
    cash_transaction_id TEXT NOT NULL,
    UNIQUE (cash_transaction_id)
);

CREATE INDEX IF NOT EXISTS paper_executions_occurred_at_idx
    ON paper_executions (occurred_at DESC, id DESC);
