"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import styles from "./page.module.css";

type WatchlistSymbol = {
  symbol: string;
  added_at: string;
};

type Watchlist = {
  id: string;
  name: string;
  symbols: WatchlistSymbol[];
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export default function WatchlistsPage() {
  const [watchlists, setWatchlists] = useState<Watchlist[]>([]);
  const [name, setName] = useState("");
  const [symbols, setSymbols] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(true);
  const [isSignedIn, setIsSignedIn] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    async function loadWatchlists() {
      try {
        const response = await fetch(`${apiURL}/api/v1/watchlists`, {
          credentials: "include",
          signal: controller.signal,
        });
        if (response.status === 401) {
          setIsSignedIn(false);
          return;
        }
        if (!response.ok) {
          setError("We could not load your watchlists.");
          return;
        }
        setWatchlists((await response.json()) as Watchlist[]);
      } catch {
        if (!controller.signal.aborted) {
          setError("TradeMind could not reach the watchlist service.");
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      }
    }

    void loadWatchlists();
    return () => controller.abort();
  }, []);

  async function createWatchlist(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError("Give your watchlist a name.");
      return;
    }
    setError(null);
    try {
      const response = await fetch(`${apiURL}/api/v1/watchlists`, {
        body: JSON.stringify({ name: trimmedName }),
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        method: "POST",
      });
      if (!response.ok) {
        setError("We could not create that watchlist.");
        return;
      }
      const watchlist = (await response.json()) as Watchlist;
      setWatchlists((current) => [watchlist, ...current]);
      setName("");
    } catch {
      setError("TradeMind could not reach the watchlist service.");
    }
  }

  async function addSymbol(event: FormEvent<HTMLFormElement>, watchlistID: string) {
    event.preventDefault();
    const symbol = symbols[watchlistID]?.trim();
    if (!symbol) {
      setError("Enter a stock symbol.");
      return;
    }
    setError(null);
    try {
      const response = await fetch(`${apiURL}/api/v1/watchlists/${watchlistID}/symbols`, {
        body: JSON.stringify({ symbol }),
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        method: "POST",
      });
      if (!response.ok) {
        setError(
          response.status === 409
            ? `${symbol.toUpperCase()} is already on this watchlist.`
            : "We could not add that symbol.",
        );
        return;
      }
      const addedSymbol = (await response.json()) as WatchlistSymbol;
      setWatchlists((current) =>
        current.map((watchlist) =>
          watchlist.id === watchlistID
            ? { ...watchlist, symbols: [...watchlist.symbols, addedSymbol] }
            : watchlist,
        ),
      );
      setSymbols((current) => ({ ...current, [watchlistID]: "" }));
    } catch {
      setError("TradeMind could not reach the watchlist service.");
    }
  }

  async function removeSymbol(watchlistID: string, symbol: string) {
    setError(null);
    try {
      const response = await fetch(
        `${apiURL}/api/v1/watchlists/${watchlistID}/symbols/${encodeURIComponent(symbol)}`,
        { credentials: "include", method: "DELETE" },
      );
      if (!response.ok) {
        setError("We could not remove that symbol.");
        return;
      }
      setWatchlists((current) =>
        current.map((watchlist) =>
          watchlist.id === watchlistID
            ? {
                ...watchlist,
                symbols: watchlist.symbols.filter((item) => item.symbol !== symbol),
              }
            : watchlist,
        ),
      );
    } catch {
      setError("TradeMind could not reach the watchlist service.");
    }
  }

  if (isLoading) {
    return <main className={styles.page}>Loading your watchlists...</main>;
  }

  if (!isSignedIn) {
    return (
      <main className={styles.page}>
        <section className={styles.emptyState}>
          <p className={styles.eyebrow}>Paper trading</p>
          <h1>Sign in to save market ideas.</h1>
          <a className={styles.primaryAction} href={`${apiURL}/api/v1/auth/google`}>
            Sign in with Google
          </a>
        </section>
      </main>
    );
  }

  return (
    <main className={styles.page}>
      <nav className={styles.nav}>
        <Link className={styles.brand} href="/">
          TradeMind
        </Link>
        <Link className={styles.dashboardLink} href="/dashboard">
          Your dashboard
        </Link>
      </nav>

      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your market ideas</p>
        <h1>Watch what matters to you.</h1>
        <p>
          Save US equity symbols to revisit them before placing a simulated trade.
        </p>
      </header>

      <form className={styles.createForm} onSubmit={createWatchlist}>
        <label className={styles.srOnly} htmlFor="watchlist-name">
          Watchlist name
        </label>
        <input
          id="watchlist-name"
          maxLength={80}
          onChange={(event) => setName(event.target.value)}
          placeholder="e.g. Long-term ideas"
          value={name}
        />
        <button type="submit">Create watchlist</button>
      </form>
      {error ? <p className={styles.error}>{error}</p> : null}

      <section className={styles.watchlistGrid} aria-label="Your watchlists">
        {watchlists.map((watchlist) => (
          <article className={styles.watchlist} key={watchlist.id}>
            <h2>{watchlist.name}</h2>
            {watchlist.symbols.length > 0 ? (
              <ul className={styles.symbolList}>
                {watchlist.symbols.map((item) => (
                  <li key={item.symbol}>
                    <span>{item.symbol}</span>
                    <button
                      aria-label={`Remove ${item.symbol} from ${watchlist.name}`}
                      onClick={() => void removeSymbol(watchlist.id, item.symbol)}
                      type="button"
                    >
                      Remove
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.emptySymbols}>No symbols yet.</p>
            )}
            <form className={styles.symbolForm} onSubmit={(event) => addSymbol(event, watchlist.id)}>
              <label className={styles.srOnly} htmlFor={`symbol-${watchlist.id}`}>
                Add a stock symbol to {watchlist.name}
              </label>
              <input
                id={`symbol-${watchlist.id}`}
                maxLength={15}
                onChange={(event) =>
                  setSymbols((current) => ({ ...current, [watchlist.id]: event.target.value }))
                }
                placeholder="AAPL"
                value={symbols[watchlist.id] ?? ""}
              />
              <button type="submit">Add</button>
            </form>
          </article>
        ))}
      </section>

      {watchlists.length === 0 ? (
        <section className={styles.emptyState}>
          <h2>Your first watchlist starts here.</h2>
          <p>Create a list, then add the stock symbols you want to follow.</p>
        </section>
      ) : null}

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
