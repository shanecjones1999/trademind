# Paper Trading App Plan

## Build approach

Create a paper-trading platform with a Go backend, a Next.js web app, and React Native iOS and Android apps. Users receive virtual cash, submit simulated orders, and see portfolios valued against licensed real-time market data. The product will not integrate with brokerages or support real-money transfers.

## 1. Define the MVP

- Account creation and onboarding with a virtual balance, such as $100,000 USD.
- Watchlists, symbol search, quotes, charts, buy and sell flows, holdings, transaction history, and portfolio performance.
- Start with US equities; add ETFs and crypto later.
- Support market and limit orders. Execute market orders using the latest eligible quote and keep limit orders open until the price crosses the limit.
- Clearly label all screens as simulated or paper trading and display quote freshness or delay.

## 2. Select the core stack

- **Web:** Next.js and TypeScript.
- **Mobile:** React Native with Expo and TypeScript, sharing API clients, data models, and UI primitives where practical.
- **Backend:** Go service using `chi`, `gin`, or `echo`, serving the shared API, paper-trading engine, and market-data ingestion. Go is well suited to concurrent quote processing and real-time client connections.
- **Data:** PostgreSQL for users, accounts, orders, and the ledger; Redis for quotes, sessions, rate limiting, and pub/sub.
- **Realtime:** WebSockets or Server-Sent Events for quotes, order status, and portfolio updates.
- **Infrastructure:** Managed PostgreSQL and Redis, object storage for assets, error tracking, metrics, and CI/CD.

## 3. Secure licensed market data

- Use Massive as the stock-market data provider. Confirm its selected plan covers US equities, real-time data, and redistribution to web and mobile users.
- Use Massive's free previous-day aggregate endpoint for early development only; it provides end-of-day data and must be replaced before real-time trading is offered.
- Build the Massive integration behind a provider abstraction so a future vendor change does not affect trading logic.
- Connect to Massive's server-side streaming API, cache normalized quotes, then distribute only entitled data to clients.
- Handle market schedules, halts, stale quotes, splits, dividends, and provider outages.
- Do not describe quotes as real time if the selected vendor plan provides delayed data.

## 4. Implement a reliable paper-trading engine

- Maintain cash, positions, fills, and adjustments in an immutable, double-entry-style ledger.
- Validate buying power and market state on the server; never trust client-submitted prices, balances, or quantities.
- Define transparent fill rules: quote source, bid/ask versus last-trade execution, slippage assumptions, partial fills, limit-order eligibility, and after-hours behavior.
- Recalculate account equity and realized and unrealized P&L after every fill and price update.
- Require idempotency keys to prevent duplicate order submissions.

## 5. Design the user experience

- **Home:** Total equity, daily and all-time returns, buying power, and holdings.
- **Discover:** Symbol lookup, watchlists, movers, security details, charts, and quote metadata.
- **Trade ticket:** Side, order type, quantity or dollar amount, estimated cost, buying-power impact, confirmation, and order receipt.
- **Portfolio:** Allocation, position-level P&L, activity, open orders, and performance history.
- Use push notifications for fills, price alerts, and market-status changes.

## 6. Build the API and data model

Main entities:

- `users`
- `paper_accounts`
- `cash_ledger_entries`
- `orders`
- `executions`
- `positions`
- `instruments`
- `quotes`
- `watchlists`
- `price_alerts`
- `corporate_actions`

Main API areas:

- Authentication
- Instruments and quotes
- Watchlists
- Orders
- Portfolio
- Activity
- Notifications
- Administration and support

Maintain auditable order and ledger history. Derive balances from ledger entries rather than overwriting balances ad hoc.

## 7. Address security and compliance

- Authenticate users with Google OAuth through OpenID Connect. Use PKCE for web and native sign-in flows, verify Google ID tokens server-side, then create or link the user account and issue secure app sessions.
- Add email authentication and MFA support later if needed, alongside secure sessions, device management, and account deletion and export.
- Encrypt sensitive data, enforce authorization at every API boundary, rate-limit order and quote endpoints, and maintain audit logs.
- Publish terms, a privacy policy, simulated-trading disclosure, market-data attribution, and a clear "not investment advice" notice.
- Review market-data vendor agreements carefully: displaying and redistributing real-time quotes often has exchange-specific requirements.

## 8. Test and operate

- Unit-test order validation, fills, ledger calculations, corporate actions, and idempotency.
- Integration-test quote ingestion, real-time updates, and mobile and web order flows.
- Load-test market-open quote volume.
- Add observability for stale feeds, order failures, latency, and portfolio-calculation errors.
- Provide admin controls to reset paper accounts, suspend abuse, replay failed jobs, and investigate account activity.

## 9. Deliver in phases

### Phase 1: Foundation

- [x] Product specifications
- [ ] Market-data licensing decision
- [x] Initial web design system and landing/dashboard shells
- [x] Core paper-account ledger domain model with exact-cent USD balances and balanced opening-cash transactions
- [x] PostgreSQL-backed paper accounts and immutable cash-ledger persistence, including automatic schema setup and $100,000 account provisioning on sign-in
- [x] Google OpenID Connect authentication and signed browser sessions (deployment configuration pending)

### Phase 2: Market data and web portfolio

- [x] Development quote ingestion through Massive's previous-day aggregate endpoint
- [ ] Licensed real-time quote ingestion, caching, and freshness handling
- [~] Exact-symbol lookup through development quote API
- [~] Web paper-account overview with virtual cash, portfolio-value placeholder, account status, market snapshot, and sign-out
- [~] Holdings and portfolio performance history
- [x] Persistent watchlists for US equity symbols

### Phase 3: Web MVP trading

- [~] Exact-cent order validation and fill-rule domain
- [~] Ledger and fills
- Portfolio performance
- Web MVP release

### Phase 4: Mobile apps

- React Native application using the shared APIs
- Push notifications
- App Store and Google Play readiness

### Phase 5: Expansion and hardening

- Limit orders
- Price alerts
- Corporate actions
- Social and leaderboard features
- Analytics
- Scaling hardening

## Recommended starting point

Start with a web-first MVP and a single market-data vendor. Design the backend as the single source of truth for both web and mobile clients.
