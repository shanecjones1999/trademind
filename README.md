# TradeMind API

The first backend service for TradeMind's paper-trading platform. It provides a
health endpoint and a server-side proxy for Massive's previous-day stock bar.
Massive credentials are never sent to web or mobile clients.

## Prerequisites

- Go 1.22 or newer
- A Massive Stocks Basic account or higher
- PostgreSQL 14 or newer for paper-account persistence

## Run locally

1. Copy `.env.example` to `.env`, set `MASSIVE_API_KEY`, and configure the
   local database connection:

   ```sh
   DATABASE_URL=postgres://trademind:trademind@localhost:5432/trademind?sslmode=disable
   ```

2. Create a local PostgreSQL container (only needed once):

   ```sh
   docker run --name trademind-postgres \
     -e POSTGRES_USER=trademind \
     -e POSTGRES_PASSWORD=trademind \
     -e POSTGRES_DB=trademind \
     -p 5432:5432 \
     -d postgres:16
   ```

3. Start the database and API together:

   ```sh
   docker start trademind-postgres && go run ./cmd/api
   ```

   The first command starts PostgreSQL in the background; the API then applies
   its database schema and listens on `:8080` by default. When running the
   database container for the first time, use the previous `docker run` command
   instead of `docker start`.

4. Bootstrap the local ticker catalog a page at a time:

   ```sh
   go run ./cmd/ticker-sync
   ```

   Run the command repeatedly (or schedule it) until each exchange/type scope
   completes. It resumes its persisted Massive pagination cursor, imports one
   page by default, and then advances to the next incomplete scope. Use
   `go run ./cmd/ticker-sync -scope XNAS:CS -pages 5` to import up to five
   NASDAQ common-stock pages in one invocation.

5. Populate the local quote store:

   ```sh
   go run ./cmd/quote-sync
   ```

   This fetches one grouped end-of-day request from Massive covering every
   symbol and upserts it into Postgres. Run it once after each trading day
   (schedule it, for example daily after market close) so `/api/v1/quotes`
   never has to call Massive on the request path.

## Web dashboard

Start the API first, then in a second terminal:

```sh
cd web
cp .env.example .env.local
npm run dev
```

The dashboard is available at `http://localhost:3000`. Its server-rendered
quote card reads the Go API at `API_URL`, which defaults to
`http://localhost:8080`.

## Endpoints

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/quotes/AAPL
curl "http://localhost:8080/api/v1/quotes?symbols=AAPL,MSFT,NVDA"
curl "http://localhost:8080/api/v1/tickers?search=apple&limit=12"
curl --cookie "trademind_session=..." \
  --header "Content-Type: application/json" \
  --data '{"side":"buy","symbol":"AAPL","quantity":2}' \
  http://localhost:8080/api/v1/orders
curl --cookie "trademind_session=..." \
  --header "Content-Type: application/json" \
  --data '{"side":"sell","symbol":"AAPL","quantity":1}' \
  http://localhost:8080/api/v1/orders
curl --cookie "trademind_session=..." \
  "http://localhost:8080/api/v1/orders?limit=25&offset=0"
```

`GET /api/v1/quotes/{symbol}` and `GET /api/v1/quotes?symbols=AAPL,MSFT,NVDA`
return the previous trading day's close, change, and timestamp metadata for up
to 12 unique, valid US equity symbols. When `DATABASE_URL` is set, both read
from the local quote store populated by `quote-sync` and never call Massive in
the user request path. Without `DATABASE_URL`, the API falls back to calling
Massive live (subject to its rate limits); run `quote-sync` and set
`DATABASE_URL` to avoid that.

`GET /api/v1/tickers` searches the local active instrument catalog and never
calls Massive in the user request path. It accepts an optional `search` term
and returns up to 12 tickers with their company names; run `ticker-sync` to
populate it before using the endpoint.

This development data is end-of-day only. Before launch, replace it with
Massive's real-time snapshot or WebSocket feed under an appropriately licensed
plan.

When `DATABASE_URL` is set, the API applies its initial paper-account schema on
startup. A successful Google sign-in then creates one paper account with
$100,000.00 in virtual cash. Signed-in users can retrieve that account with
`GET /api/v1/account`, including any open paper positions, and their filled
trades with `GET /api/v1/orders`.

## Instrument catalog synchronization

TradeMind keeps a local PostgreSQL catalog of active and historical US
instruments. This is reference data, not a cache of quotes: ticker search
and order symbol validation query the local
catalog. Quotes are served from a separate local store (see `quote-sync`
above) populated from the market-data provider on a schedule, not fetched live
per request.

### Initial coverage

The initial universe is US-listed common stocks and ETFs whose primary listing
is on NASDAQ, NYSE, NYSE American, NYSE Arca, or a Cboe US equities exchange.
Do not ingest OTC or pink-sheet securities, foreign primary listings,
cryptocurrencies, options, warrants, or other derivative instruments in this
phase. Each catalog record must retain its primary exchange and asset type so
the universe can expand later without a schema change.

### Implementation plan

1. Add an idempotent PostgreSQL migration for an `instruments` table, applied
   with the existing embedded migrations. Store a normalized symbol and the
   provider's stable identifiers (for example, composite and share-class FIGI)
   alongside name, asset type, primary-exchange MIC, active status,
   provider-update time, first-seen time, last-seen time, and delisting time.
   Make the stable provider identifier unique when present; use the normalized
   symbol plus primary exchange as the listing key. Index active-symbol exact
   lookups and active name/symbol search.
2. Introduce a catalog repository with transactional, batched upserts and
   search methods. The public ticker endpoint must use this repository only;
   do not call the provider synchronously when a user searches. Return active
   listings by default and retain inactive/delisted records for history.
3. Add a dedicated, authenticated server-side sync command or scheduled job.
   It should call Massive's reference-tickers endpoint for each allowed
   exchange and asset type, follow every pagination URL, normalize provider
   values, and upsert each page. Its initial run is the bootstrap; subsequent
   full refreshes run daily outside market hours.
4. Mark a listing inactive only after a complete, successful refresh of its
   relevant source scope. Never delete catalog rows merely because an
   individual page, exchange request, or entire run failed. Persist a sync-run
   status and counts so a failed or partial run is visible and can be retried.
5. Update order validation to accept only active
   instruments in the initial universe.
6. Test pagination, exchange/type filtering, normalization, idempotent
   upserts, delisting, partial-run safety, and local endpoint search. Emit
   structured logs and metrics for run duration, pages, inserts, updates,
   deactivations, and provider failures.

The sync job requires `MASSIVE_API_KEY` and must respect the selected Massive
plan's reference-data entitlements and rate limits. It should use the
provider's `next_url` pagination cursor rather than attempting to retrieve the
entire universe in one request.

## Paper-order fill rules

The domain layer now defines exact-cent paper-order validation and fills. It
executes buys against the ask and sells against the bid, applies a configurable
slippage allowance, rejects stale quotes, enforces market and limit-order
eligibility, and verifies buying power or available shares before producing a
balanced cash-ledger transaction. Completed orders are applied to FIFO position
lots, preserving cost basis and realized P&L at cent precision.

`POST /api/v1/orders` now accepts authenticated market buy and sell orders with
an order side, stock symbol, and whole-share quantity. `GET /api/v1/orders`
returns the signed-in user's filled orders newest first, including fill price,
cash impact, and realized P&L on sells. It accepts `limit` (1–200, default 50)
and `offset` (0-based) and includes `total` so clients can paginate the full
history. In development, orders
execute against the latest delayed quote returned by the configured
market-data provider, using the quote price when bid and ask data are
unavailable.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated web origins |
| `MASSIVE_API_KEY` | none | Server-side Massive API key |
| `DATABASE_URL` | none | PostgreSQL connection string for paper accounts and cash ledger |
| `GOOGLE_CLIENT_ID` | none | Google OpenID Connect client ID |
| `GOOGLE_CLIENT_SECRET` | none | Google OpenID Connect client secret |
| `GOOGLE_REDIRECT_URL` | `http://localhost:8080/api/v1/auth/google/callback` | Registered Google OAuth callback |
| `APP_WEB_URL` | `http://localhost:3000/dashboard` | Web app URL after successful sign-in |
| `AUTH_SESSION_SECRET` | none | At least 32 random bytes used to sign sessions |

The service loads a local `.env` file automatically without overriding
environment variables supplied by the deployment platform.

## Google sign-in

When all Google and session variables are configured, the API enables:

- `GET /api/v1/auth/google` to start Google sign-in
- `GET /api/v1/auth/google/callback` for the registered OAuth callback
- `GET /api/v1/me` to retrieve the signed-in profile
- `POST /api/v1/auth/logout` to clear the session

Set `GOOGLE_REDIRECT_URL` exactly to the authorized redirect URI in Google Cloud
Console. Generate `AUTH_SESSION_SECRET` with `openssl rand -base64 48`; do not
reuse the Google client secret.

## Validation

```sh
go test ./cmd/... ./internal/...
```
