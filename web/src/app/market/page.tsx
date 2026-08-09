"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { currency, type Quote } from "@/lib/market";
import styles from "./page.module.css";

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const tickerListLimit = 12;

type Ticker = {
  symbol: string;
  name: string;
};

export default function MarketPage() {
  const [query, setQuery] = useState("AAPL");
  const [quote, setQuote] = useState<Quote | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
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

  async function lookupSymbol(symbol: string) {
    const normalizedSymbol = symbol.trim();
    if (!normalizedSymbol) {
      setQuote(null);
      setMessage("Enter a US equity symbol to look it up.");
      return;
    }

    setIsLoading(true);
    setMessage(null);
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

  const changeIsPositive = quote ? quote.day_change >= 0 : true;

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
          Enter a US equity symbol to see Massive&apos;s latest available
          end-of-day bar.
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
          Development data is delayed to the previous trading day and is not
          eligible for simulated order execution.
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
                  <div className={styles.tableRow} key={ticker.symbol} role="row">
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
                  </div>
                );
              })}
            </div>
          )}
        </section>

        {quote ? (
          <article className={styles.quoteCard} aria-live="polite">
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
        ) : (
          <article className={styles.unavailable} aria-live="polite">
            {message ?? "Enter a symbol to retrieve a quote."}
          </article>
        )}
      </section>
    </main>
  );
}
