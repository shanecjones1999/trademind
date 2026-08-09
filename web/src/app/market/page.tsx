"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { currency, currencyFromCents, type Quote } from "@/lib/market";
import styles from "./page.module.css";

type Ticker = {
  symbol: string;
  name: string;
};

type Position = {
  symbol: string;
  quantity: number;
  cost_basis_cents: number;
  realized_pnl_cents: number;
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
    };
  };
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const tickerListLimit = 12;

export default function MarketPage() {
  const [query, setQuery] = useState("AAPL");
  const [quote, setQuote] = useState<Quote | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [accountMessage, setAccountMessage] = useState<string | null>(null);
  const [isSignedIn, setIsSignedIn] = useState(true);
  const [orderSide, setOrderSide] = useState<"buy" | "sell">("buy");
  const [orderQuantity, setOrderQuantity] = useState("1");
  const [orderError, setOrderError] = useState<string | null>(null);
  const [orderMessage, setOrderMessage] = useState<string | null>(null);
  const [isSubmittingOrder, setIsSubmittingOrder] = useState(false);
  const [tickerSearch, setTickerSearch] = useState("");
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
      setQuote(null);
      setMessage("Enter a US equity symbol to look it up.");
      setOrderError(null);
      setOrderMessage(null);
      return;
    }

    setIsLoading(true);
    setMessage(null);
    setOrderError(null);
    setOrderMessage(null);
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

  function submitLookup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void lookupSymbol(query);
  }

  async function submitOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!quote) {
      setOrderError("Look up a stock before placing a paper trade.");
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
    setOrderMessage(null);
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
        setOrderError(
          response.status === 409
            ? orderSide === "buy"
              ? "You do not have enough virtual cash for that order."
              : "You do not own enough shares for that order."
            : "We could not place that paper trade.",
        );
        return;
      }

      const payload = (await response.json()) as OrderResponse;
      setAccount(payload.account);
      setOrderMessage(
        `${payload.fill.order.side === "buy" ? "Bought" : "Sold"} ${payload.fill.order.quantity} share${
          payload.fill.order.quantity === 1 ? "" : "s"
        } of ${payload.fill.order.symbol} at ${currencyFromCents(payload.fill.execution.price_cents)}.`,
      );
    } catch {
      setOrderError("TradeMind could not reach the paper-trading service.");
    } finally {
      setIsSubmittingOrder(false);
    }
  }

  const changeIsPositive = quote ? quote.day_change >= 0 : true;
  const buyingPower = account ? currencyFromCents(account.cash_balance_cents) : null;
  const selectedPosition =
    account?.positions.find((position) => position.symbol === quote?.symbol) ?? null;

  return (
    <main className={styles.page}>
      <nav className={styles.nav}>
        <Link className={styles.brand} href="/">
          TradeMind
        </Link>
        <div className={styles.navLinks}>
          <Link className={styles.textLink} href="/watchlists">
            Watchlists
          </Link>
          <Link className={styles.dashboardLink} href="/dashboard">
            Your dashboard
          </Link>
        </div>
      </nav>

      <section className={styles.content}>
        <p className={styles.eyebrow}>Market data</p>
        <h1>Look up a stock before you trade.</h1>
        <p className={styles.description}>
          Enter a US equity symbol to review the latest available price and
          place paper buy or sell orders.
        </p>

        <form className={styles.lookupForm} onSubmit={submitLookup}>
          <label className={styles.srOnly} htmlFor="symbol">
            US equity symbol
          </label>
          <input
            autoCapitalize="characters"
            autoComplete="off"
            id="symbol"
            maxLength={15}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="AAPL"
            value={query}
          />
          <button disabled={isLoading} type="submit">
            {isLoading ? "Loading..." : "Get quote"}
          </button>
        </form>

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
              {tickers.map((ticker) => {
                const featuredQuote = featuredQuotes.find(
                  ({ symbol }) => symbol === ticker.symbol,
                );
                const isPositive = featuredQuote ? featuredQuote.day_change >= 0 : true;
                return (
                  <button
                    className={styles.tableRow}
                    key={ticker.symbol}
                    onClick={() => {
                      setQuery(ticker.symbol);
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
                      onChange={(event) => setOrderSide(event.target.value as "buy" | "sell")}
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
                      onChange={(event) => setOrderQuantity(event.target.value)}
                      step={1}
                      type="number"
                      value={orderQuantity}
                    />
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
              {orderMessage ? <p className={styles.orderSuccess}>{orderMessage}</p> : null}
            </aside>
          </section>
        ) : (
          <article className={styles.unavailable} aria-live="polite">
            {message ?? "Enter a symbol to retrieve a quote."}
          </article>
        )}
      </section>
    </main>
  );
}
