# TradeMind API

The first backend service for TradeMind's paper-trading platform. It provides a
health endpoint and a server-side proxy for Massive's previous-day stock bar.
Massive credentials are never sent to web or mobile clients.

## Prerequisites

- Go 1.22 or newer
- A Massive Stocks Basic account or higher
- PostgreSQL 14 or newer for paper-account persistence

## Run locally

1. Copy `.env.example` to `.env` and set `MASSIVE_API_KEY`.
2. Run:

   ```sh
   go run ./cmd/api
   ```

The server listens on `:8080` by default.

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
```

`GET /api/v1/quotes/{symbol}` requests Massive's free-tier previous-day bar
endpoint from the backend and returns the previous trading day's close, change,
and timestamp metadata. It requires `MASSIVE_API_KEY`.

`GET /api/v1/quotes?symbols=AAPL,MSFT,NVDA` returns up to 12 unique, valid US
equity quotes in request order. Batch requests use one cached grouped
end-of-day market-data request rather than one upstream request per symbol.

`GET /api/v1/tickers` searches Massive's active US common-stock catalog. It
accepts an optional `search` term and returns up to 12 tickers with their
company names; the market page uses it to browse and filter its price list.

This development endpoint is end-of-day only. Before launch, replace it with
Massive's real-time snapshot or WebSocket feed under an appropriately licensed
plan.

When `DATABASE_URL` is set, the API applies its initial paper-account schema on
startup. A successful Google sign-in then creates one paper account with
$100,000.00 in virtual cash. Signed-in users can retrieve that account with
`GET /api/v1/account`.

Signed-in users can also organize US equity symbols in persistent watchlists:

```sh
curl --cookie "trademind_session=..." http://localhost:8080/api/v1/watchlists
curl --cookie "trademind_session=..." \
  --header "Content-Type: application/json" \
  --data '{"name":"Long-term ideas"}' \
  http://localhost:8080/api/v1/watchlists
curl --cookie "trademind_session=..." \
  --header "Content-Type: application/json" \
  --data '{"symbol":"AAPL"}' \
  http://localhost:8080/api/v1/watchlists/{watchlist_id}/symbols
```

## Paper-order fill rules

The domain layer now defines exact-cent paper-order validation and fills. It
executes buys against the ask and sells against the bid, applies a configurable
slippage allowance, rejects stale quotes, enforces market and limit-order
eligibility, and verifies buying power or available shares before producing a
balanced cash-ledger transaction. Completed orders are applied to FIFO position
lots, preserving cost basis and realized P&L at cent precision.

This is not exposed as a trading endpoint yet: the current Massive integration
is previous-day data and is not eligible to execute simulated orders. A
licensed, fresh bid/ask feed is required before wiring these rules to the API.

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
