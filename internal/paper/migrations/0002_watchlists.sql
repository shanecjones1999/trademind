CREATE TABLE IF NOT EXISTS watchlists (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS watchlist_symbols (
    watchlist_id TEXT NOT NULL REFERENCES watchlists (id) ON DELETE CASCADE,
    symbol TEXT NOT NULL,
    added_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (watchlist_id, symbol)
);

CREATE INDEX IF NOT EXISTS watchlists_user_created_idx
    ON watchlists (user_id, created_at DESC);
