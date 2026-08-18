# Feature backlog

Prioritized for the web paper-trading MVP. Real-time prices come first: fills, portfolio value, and resting orders still run off yesterday’s close from `quote-sync`. Limit and stop orders cannot behave correctly until prices move during the day.

## Next

1. **Real-time stock prices** (critical)
   Unblocks honest P&L and every non-market order. Do licensing plus Massive snapshot/WebSocket, then cache and push to clients. Show quote freshness on every screen. Remaining Phase 2 work in the product plan.

2. **Limit orders** — only buy/sell at the specified price or better.
   Already in the MVP spec and domain (`OrderTypeLimit`). The HTTP API still only creates market fills. Needs a resting-order store and a matcher on quote updates.

3. **Fractional shares**
   Independent of live quotes. High practice value on expensive names. Quantity is still `int64` whole shares, so this is a ledger/position precision change, not a new product surface.

## After the matcher exists

4. **Stop orders**, then **stop-limit orders**
   Reuse the same open-order + quote-driven fill path as limits. Stops are the useful risk-management type; stop-limits are a small extra on top.

## Later

5. **Dividends + DRIP**
   Makes long-hold P&L honest, but this is corporate-actions work (ex-date cash credit, optional auto-buy). It does not improve the trade ticket. Start with cash dividends only; add reinvestment after that is reliable.

6. **Groups**
   Lowest leverage unless this means something named folders, portfolio sleeves, or similar. Clarify before building.

7. **Shorting / puts**
   Highest cost, deferred in the product plan. Shorts need margin and borrow rules; puts need an options catalog, chain, and a different ledger. Do this after the long-only equity engine is solid.

## Done

- **Market orders** — execute at the current market price. Live as whole-share buy/sell against the current quote. Do not spend a cycle on them except to wire `order_type` once limits land.

## Suggested sequence

1. Real-time quotes (and quote freshness on every screen)
2. Persist open orders + fill limits when price crosses
3. Fractional quantity
4. Stops, then stop-limits
5. Cash dividends, then optional DRIP
6. Groups, shorts, options — only with a clear user story

If there is a single next ticket, it is real-time quote ingestion and freshness, not another order type.
