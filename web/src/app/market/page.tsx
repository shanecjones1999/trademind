"use client";

import { FormEvent, useEffect, useState } from "react";
import Nav from "@/app/_components/Nav";
import { currency, currencyFromCents, type Quote } from "@/lib/market";
import type { Position } from "@/lib/portfolio";
import styles from "./page.module.css";

type Ticker = {
  symbol: string;
  name: string;
};

type AccountSnapshot = {
  account: {
    id: string;
    user_id: string;
    opened_at: string;
  };
  cash_balance_cents: number;
  positions: Position[];
};

type OrderResponse = {
  account: AccountSnapshot;
  fill: {
    order: {
      side: "buy" | "sell";
      symbol: string;
      quantity: number;
    };
    execution: {
      price_cents: number;
      occurred_at: string;
    };
  };
};

type Receipt = {
  side: "buy" | "sell";
  symbol: string;
  quantity: number;
  priceCents: number;
  occurredAt: string;
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const tickerListLimit = 12;

async function errorMessageFromResponse(response: Response): Promise<string | null> {
  try {
    const payload = (await response.json()) as { error?: unknown };
    if (typeof payload.error !== "string") {
      return null;
    }
    const message = payload.error.trim();
    if (!message) {
      return null;
    }
    return message.charAt(0).toUpperCase() + message.slice(1);
  } catch {
    return null;
  }
}

export default function MarketPage() {
  const [quote, setQuote] = useState<Quote | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [accountMessage, setAccountMessage] = useState<string | null>(null);
  const [isSignedIn, setIsSignedIn] = useState(true);
  const [orderSide, setOrderSide] = useState<"buy" | "sell">("buy");
  const [orderQuantity, setOrderQuantity] = useState("1");
  const [orderError, setOrderError] = useState<string | null>(null);
  const [receipt, setReceipt] = useState<Receipt | null>(null);
  const [isSubmittingOrder, setIsSubmittingOrder] = useState(false);
  const [tickerSearch, setTickerSearch] = useState("");
  const [tickerSort, setTickerSort] = useState<"symbol" | "change">("symbol");
  const [tickers, setTickers] = useState<Ticker[]>([]);
  const [featuredQuotes, setFeaturedQuotes] = useState<Quote[]>([]);
  const [featuredError, setFeaturedError] = useState<string | null>(null);
  const [isLoadingFeatured, setIsLoadingFeatured] = useState(true);

  useEffect(() => {
    const controller = new AbortController();

    async function loadTickers() {
      setIsLoadingFeatured(true);
      setFeaturedError(null);
      try {
        const query = new URLSearchParams({ limit: String(tickerListLimit) });
        if (tickerSearch.trim()) {
          query.set("search", tickerSearch.trim());
        }
        const tickerResponse = await fetch(`${apiURL}/api/v1/tickers?${query}`, {
          signal: controller.signal,
        });
        if (!tickerResponse.ok) {
          setFeaturedError("Stock listings are temporarily unavailable.");
          return;
        }
        const listedTickers = (await tickerResponse.json()) as Ticker[];
        setTickers(listedTickers);
        if (listedTickers.length === 0) {
          setFeaturedQuotes([]);
          return;
        }

        const symbols = listedTickers.map(({ symbol }) => symbol).join(",");
        const quoteResponse = await fetch(`${apiURL}/api/v1/quotes?symbols=${symbols}`, {
          signal: controller.signal,
        });
        if (!quoteResponse.ok) {
          setFeaturedError("Market prices are temporarily unavailable.");
          return;
        }
        setFeaturedQuotes((await quoteResponse.json()) as Quote[]);
      } catch {
        if (!controller.signal.aborted) {
          setFeaturedError("TradeMind could not reach the market-data service.");
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoadingFeatured(false);
        }
      }
    }

    const timeout = window.setTimeout(() => void loadTickers(), 300);
    return () => {
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [tickerSearch]);

  useEffect(() => {
    const controller = new AbortController();

    async function loadAccount() {
      try {
        const response = await fetch(`${apiURL}/api/v1/account`, {
          credentials: "include",
          signal: controller.signal,
        });
        if (response.status === 401) {
          setIsSignedIn(false);
          setAccount(null);
          setAccountMessage(null);
          return;
        }
        if (!response.ok) {
          setAccountMessage("Paper buying power is temporarily unavailable.");
          return;
        }
        setIsSignedIn(true);
        setAccountMessage(null);
        setAccount((await response.json()) as AccountSnapshot);
      } catch {
        if (!controller.signal.aborted) {
          setAccountMessage("TradeMind could not reach the paper-trading service.");
        }
      }
    }

    void loadAccount();
    return () => controller.abort();
  }, []);

  async function lookupSymbol(symbol: string) {
    const normalizedSymbol = symbol.trim();
    if (!normalizedSymbol) {
      return;
    }

    setIsLoading(true);
    setMessage(null);
    setOrderError(null);
    setReceipt(null);
    try {
      const response = await fetch(
        `${apiURL}/api/v1/quotes/${encodeURIComponent(normalizedSymbol)}`,
      );
      if (response.status === 404) {
        setQuote(null);
        setMessage(`We could not find "${normalizedSymbol.toUpperCase()}".`);
        return;
      }
      if (!response.ok) {
        setQuote(null);
        setMessage("Market data is temporarily unavailable. Please try again.");
        return;
      }
      setQuote((await response.json()) as Quote);
    } catch {
      setQuote(null);
      setMessage("TradeMind could not reach the market-data service.");
    } finally {
      setIsLoading(false);
    }
  }

  async function submitOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!quote) {
      setOrderError("Select a stock before placing a paper trade.");
      return;
    }

    const quantity = Number.parseInt(orderQuantity, 10);
    if (!Number.isInteger(quantity) || quantity <= 0) {
      setOrderError("Enter a whole-share quantity to trade.");
      return;
    }
    if (!account) {
      setOrderError(
        isSignedIn
          ? "Your buying power is still loading. Please try again."
          : "Sign in to place a paper trade.",
      );
      return;
    }

    setIsSubmittingOrder(true);
    setOrderError(null);
    setReceipt(null);
    try {
      const response = await fetch(`${apiURL}/api/v1/orders`, {
        body: JSON.stringify({ side: orderSide, symbol: quote.symbol, quantity }),
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        method: "POST",
      });
      if (response.status === 401) {
        setIsSignedIn(false);
        setAccount(null);
        setOrderError("Sign in to place a paper trade.");
        return;
      }
      if (!response.ok) {
        if (response.status === 409) {
          setOrderError(
            orderSide === "buy"
              ? "You do not have enough virtual cash for that order."
              : "You do not own enough shares for that order.",
          );
          return;
        }
        setOrderError(
          (await errorMessageFromResponse(response)) ??
            "We could not place that paper trade.",
        );
        return;
      }

      const payload = (await response.json()) as OrderResponse;
      setAccount(payload.account);
      setReceipt({
        side: payload.fill.order.side,
        symbol: payload.fill.order.symbol,
        quantity: payload.fill.order.quantity,
        priceCents: payload.fill.execution.price_cents,
        occurredAt: payload.fill.execution.occurred_at,
      });
    } catch {
      setOrderError("TradeMind could not reach the paper-trading service.");
    } finally {
      setIsSubmittingOrder(false);
    }
  }

  const sortedTickers = [...tickers].sort((a, b) => {
    if (tickerSort === "symbol") {
      return a.symbol.localeCompare(b.symbol);
    }
    const aChange = featuredQuotes.find(({ symbol }) => symbol === a.symbol)?.day_change_pct;
    const bChange = featuredQuotes.find(({ symbol }) => symbol === b.symbol)?.day_change_pct;
    return (bChange ?? -Infinity) - (aChange ?? -Infinity);
  });

  const changeIsPositive = quote ? quote.day_change >= 0 : true;
  const buyingPower = account ? currencyFromCents(account.cash_balance_cents) : null;
  const selectedPosition =
    account?.positions.find((position) => position.symbol === quote?.symbol) ?? null;
  const ownedShares = selectedPosition?.quantity ?? 0;

  const parsedQuantity = Number.parseInt(orderQuantity, 10);
  const previewQuantity =
    Number.isInteger(parsedQuantity) && parsedQuantity > 0 ? parsedQuantity : 0;
  const estimatedCostCents =
    quote && previewQuantity > 0 ? Math.round(quote.price * 100) * previewQuantity : 0;
  const projectedCashCents =
    account && previewQuantity > 0
      ? orderSide === "buy"
        ? account.cash_balance_cents - estimatedCostCents
        : account.cash_balance_cents + estimatedCostCents
      : null;
  const remainingShares =
    previewQuantity > 0
      ? orderSide === "buy"
        ? ownedShares + previewQuantity
        : ownedShares - previewQuantity
      : null;
  const projectedShares =
    remainingShares === null ? null : Math.max(0, remainingShares);
  const exceedsBuyingPower =
    orderSide === "buy" && projectedCashCents !== null && projectedCashCents < 0;
  const exceedsPosition =
    orderSide === "sell" && remainingShares !== null && remainingShares < 0;

  return (
    <main className={styles.page}>
      <Nav active="market" />

      <section className={styles.content}>
        <p className={styles.eyebrow}>Market data</p>
        <h1>Browse prices, then trade.</h1>
        <p className={styles.description}>
          Search the US equity directory and select a stock to review the
          latest available price and place a paper buy or sell.
        </p>

        <p className={styles.disclosure}>
          Paper trades execute against the latest delayed quote available in
          development.
        </p>

        <section className={styles.priceList} aria-labelledby="stock-list-title">
          <div className={styles.listHeading}>
            <div>
              <p className={styles.eyebrow}>Stock directory</p>
              <h2 id="stock-list-title">Browse stock prices.</h2>
            </div>
            <span>Previous close</span>
          </div>
          <div className={styles.tickerControls}>
            <div className={styles.tickerSearch}>
              <label htmlFor="ticker-search">Filter by ticker or company</label>
              <input
                autoComplete="off"
                id="ticker-search"
                onChange={(event) => setTickerSearch(event.target.value)}
                placeholder="e.g. Apple or AAPL"
                value={tickerSearch}
              />
            </div>
            <div className={styles.tickerSortControl}>
              <label htmlFor="ticker-sort">Sort by</label>
              <select
                id="ticker-sort"
                onChange={(event) => setTickerSort(event.target.value as "symbol" | "change")}
                value={tickerSort}
              >
                <option value="symbol">Ticker</option>
                <option value="change">Day change</option>
              </select>
            </div>
          </div>
          {isLoadingFeatured ? (
            <p className={styles.listStatus}>Loading stock listings...</p>
          ) : featuredError ? (
            <p className={styles.listStatus}>{featuredError}</p>
          ) : tickers.length === 0 ? (
            <p className={styles.listStatus}>No active US stocks match that filter.</p>
          ) : (
            <div className={styles.quoteTable} role="table" aria-label="Popular stock prices">
              <div className={styles.tableHeader} role="row">
                <span role="columnheader">Stock</span>
                <span role="columnheader">Price</span>
                <span role="columnheader">Day change</span>
              </div>
              {sortedTickers.map((ticker) => {
                const featuredQuote = featuredQuotes.find(
                  ({ symbol }) => symbol === ticker.symbol,
                );
                const isPositive = featuredQuote ? featuredQuote.day_change >= 0 : true;
                return (
                  <button
                    className={styles.tableRow}
                    key={ticker.symbol}
                    onClick={() => {
                      void lookupSymbol(ticker.symbol);
                    }}
                    role="row"
                    type="button"
                  >
                    <span role="cell">
                      <strong>{ticker.symbol}</strong>
                      <small>{ticker.name}</small>
                    </span>
                    <span role="cell">
                      {featuredQuote ? currency(featuredQuote.price) : "Unavailable"}
                    </span>
                    <span
                      className={isPositive ? styles.positiveChange : styles.negativeChange}
                      role="cell"
                    >
                      {featuredQuote ? (
                        <>
                          {isPositive ? "+" : ""}
                          {currency(featuredQuote.day_change)} ({isPositive ? "+" : ""}
                          {featuredQuote.day_change_pct.toFixed(2)}%)
                        </>
                      ) : (
                        "Unavailable"
                      )}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </section>

        {quote ? (
          <section className={styles.tradePanel} aria-live="polite">
            <article className={styles.quoteCard}>
              <div>
                <p className={styles.symbol}>{quote.symbol}</p>
                <p className={styles.price}>{currency(quote.price)}</p>
              </div>
              <div className={styles.details}>
                <p
                  className={
                    changeIsPositive ? styles.positiveChange : styles.negativeChange
                  }
                >
                  {changeIsPositive ? "+" : ""}
                  {currency(quote.day_change)} ({changeIsPositive ? "+" : ""}
                  {quote.day_change_pct.toFixed(2)}%)
                </p>
                <p className={styles.meta}>
                  Previous close ·{" "}
                  {new Intl.DateTimeFormat("en-US", {
                    dateStyle: "medium",
                    timeZone: "UTC",
                  }).format(new Date(quote.as_of))}
                </p>
                <p className={styles.meta}>{quote.source}</p>
              </div>
            </article>

            <aside className={styles.orderCard}>
              <p className={styles.eyebrow}>Paper trade</p>
              <h2>{orderSide === "buy" ? "Buy" : "Sell"} {quote.symbol}</h2>
              <p className={styles.orderSummary}>
                {buyingPower ? `Buying power: ${buyingPower}` : "Buying power is loading."}
              </p>
              <p className={styles.meta}>
                Shares owned: {selectedPosition?.quantity ?? 0}
              </p>
              {accountMessage ? <p className={styles.meta}>{accountMessage}</p> : null}
              {isSignedIn ? (
                account ? (
                  <form className={styles.orderForm} onSubmit={submitOrder}>
                    <label htmlFor="order-side">Action</label>
                    <select
                      id="order-side"
                      onChange={(event) => {
                        setOrderSide(event.target.value as "buy" | "sell");
                        setReceipt(null);
                      }}
                      value={orderSide}
                    >
                      <option value="buy">Buy</option>
                      <option value="sell">Sell</option>
                    </select>
                    <label htmlFor="order-quantity">Shares</label>
                    <input
                      id="order-quantity"
                      inputMode="numeric"
                      min={1}
                      onChange={(event) => {
                        setOrderQuantity(event.target.value);
                        setReceipt(null);
                      }}
                      step={1}
                      type="number"
                      value={orderQuantity}
                    />
                    {previewQuantity > 0 ? (
                      <div className={styles.orderPreview}>
                        <p>
                          <span>Estimated {orderSide === "buy" ? "cost" : "proceeds"}</span>
                          <strong>{currencyFromCents(estimatedCostCents)}</strong>
                        </p>
                        <p>
                          <span>{orderSide === "buy" ? "Cash after trade" : "Cash after sale"}</span>
                          <strong className={exceedsBuyingPower ? styles.negativeChange : undefined}>
                            {projectedCashCents !== null
                              ? currencyFromCents(projectedCashCents)
                              : "Unavailable"}
                          </strong>
                        </p>
                        <p>
                          <span>Shares after trade</span>
                          <strong className={exceedsPosition ? styles.negativeChange : undefined}>
                            {projectedShares ?? "Unavailable"}
                          </strong>
                        </p>
                      </div>
                    ) : null}
                    <button disabled={isSubmittingOrder} type="submit">
                      {isSubmittingOrder
                        ? "Placing trade..."
                        : orderSide === "buy"
                          ? "Buy stock"
                          : "Sell stock"}
                    </button>
                  </form>
                ) : (
                  <p className={styles.meta}>Loading your paper account...</p>
                )
              ) : (
                <a className={styles.dashboardLink} href={`${apiURL}/api/v1/auth/google`}>
                  Sign in to buy
                </a>
              )}
              {orderError ? <p className={styles.orderError}>{orderError}</p> : null}
              {receipt ? (
                <article className={styles.receipt} aria-label="Order receipt">
                  <p className={styles.receiptTitle}>
                    {receipt.side === "buy" ? "Bought" : "Sold"} {receipt.quantity} share
                    {receipt.quantity === 1 ? "" : "s"} of {receipt.symbol}
                  </p>
                  <dl>
                    <div>
                      <dt>Execution price</dt>
                      <dd>{currencyFromCents(receipt.priceCents)}</dd>
                    </div>
                    <div>
                      <dt>Total</dt>
                      <dd>{currencyFromCents(receipt.priceCents * receipt.quantity)}</dd>
                    </div>
                    <div>
                      <dt>Executed</dt>
                      <dd>
                        {new Intl.DateTimeFormat("en-US", {
                          dateStyle: "medium",
                          timeStyle: "short",
                          timeZone: "UTC",
                        }).format(new Date(receipt.occurredAt))}{" "}
                        UTC
                      </dd>
                    </div>
                  </dl>
                </article>
              ) : null}
            </aside>
          </section>
        ) : (
          <article className={styles.unavailable} aria-live="polite">
            {isLoading
              ? "Loading quote..."
              : (message ?? "Select a stock to review the latest price and place a paper trade.")}
          </article>
        )}
      </section>
    </main>
  );
}
