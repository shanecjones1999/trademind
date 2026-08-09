CREATE TABLE IF NOT EXISTS instruments (
    provider_key TEXT PRIMARY KEY,
    scope_key TEXT NOT NULL,
    symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    primary_exchange TEXT NOT NULL,
    composite_figi TEXT,
    share_class_figi TEXT,
    active BOOLEAN NOT NULL,
    provider_updated_at TIMESTAMPTZ,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_run_id TEXT NOT NULL,
    delisted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS instruments_active_listing_idx
    ON instruments (scope_key, symbol)
    WHERE active;

CREATE INDEX IF NOT EXISTS instruments_active_symbol_idx
    ON instruments (symbol)
    WHERE active;

CREATE INDEX IF NOT EXISTS instruments_active_name_idx
    ON instruments (name)
    WHERE active;

CREATE TABLE IF NOT EXISTS instrument_sync_scopes (
    scope_key TEXT PRIMARY KEY,
    exchange TEXT NOT NULL,
    asset_type TEXT NOT NULL,
    current_run_id TEXT NOT NULL,
    next_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('running', 'completed')),
    started_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS instrument_sync_runs (
    id TEXT PRIMARY KEY,
    scope_key TEXT NOT NULL REFERENCES instrument_sync_scopes (scope_key),
    status TEXT NOT NULL CHECK (status IN ('running', 'completed')),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    page_count INTEGER NOT NULL DEFAULT 0,
    record_count INTEGER NOT NULL DEFAULT 0
);
