import type { Quote } from "./market";

export type Position = {
  symbol: string;
  quantity: number;
  cost_basis_cents: number;
  realized_pnl_cents: number;
};

export type HoldingRow = {
  symbol: string;
  quantity: number;
  avgCostCents: number;
  priceCents: number | null;
  marketValueCents: number;
  unrealizedPnLCents: number | null;
  unrealizedPnLPct: number | null;
  todayChangeCents: number | null;
  realizedPnLCents: number;
};

// Matches paper.DefaultStartingCashCents (internal/paper/ledger.go), the
// fixed virtual balance every paper account opens with.
export const STARTING_CASH_CENTS = 10_000_000;

export function quotesBySymbol(quotes: Quote[]): Record<string, Quote> {
  return Object.fromEntries(quotes.map((quote) => [quote.symbol, quote]));
}

export function buildHoldingsRows(
  positions: Position[],
  quotes: Record<string, Quote>,
): HoldingRow[] {
  return positions.map((position) => {
    const quote = quotes[position.symbol];
    const priceCents = quote ? Math.round(quote.price * 100) : null;
    const marketValueCents =
      priceCents !== null ? priceCents * position.quantity : position.cost_basis_cents;
    const unrealizedPnLCents =
      priceCents !== null ? marketValueCents - position.cost_basis_cents : null;
    const unrealizedPnLPct =
      unrealizedPnLCents !== null && position.cost_basis_cents !== 0
        ? (unrealizedPnLCents / position.cost_basis_cents) * 100
        : null;
    const todayChangeCents = quote
      ? Math.round(quote.day_change * 100) * position.quantity
      : null;

    return {
      symbol: position.symbol,
      quantity: position.quantity,
      avgCostCents:
        position.quantity !== 0
          ? Math.round(position.cost_basis_cents / position.quantity)
          : 0,
      priceCents,
      marketValueCents,
      unrealizedPnLCents,
      unrealizedPnLPct,
      todayChangeCents,
      realizedPnLCents: position.realized_pnl_cents,
    };
  });
}

export function totalMarketValueCents(rows: HoldingRow[]): number {
  return rows.reduce((total, row) => total + row.marketValueCents, 0);
}

export function totalTodayChangeCents(rows: HoldingRow[]): number {
  return rows.reduce((total, row) => total + (row.todayChangeCents ?? 0), 0);
}

export function allTimeReturnCents(totalEquityCents: number): number {
  return totalEquityCents - STARTING_CASH_CENTS;
}

export function allTimeReturnPct(totalEquityCents: number): number {
  return (allTimeReturnCents(totalEquityCents) / STARTING_CASH_CENTS) * 100;
}
