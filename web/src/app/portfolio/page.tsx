"use client";

import { useEffect, useState } from "react";
import Nav from "@/app/_components/Nav";
import AllocationBar, {
  allocationPalette,
  cashColor,
  otherColor,
  type AllocationSegment,
} from "@/app/_components/AllocationBar";
import HoldingsTable from "@/app/_components/HoldingsTable";
import { currencyFromCents, type Quote } from "@/lib/market";
import {
  allTimeReturnCents,
  allTimeReturnPct,
  buildHoldingsRows,
  quotesBySymbol,
  totalMarketValueCents,
  totalTodayChangeCents,
  type Position,
} from "@/lib/portfolio";
import styles from "./page.module.css";

type AccountSnapshot = {
  account: {
    id: string;
    user_id: string;
    opened_at: string;
  };
  cash_balance_cents: number;
  positions: Position[];
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const maxIndividualAllocationSegments = 7;

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positiveChange : styles.negativeChange;
}

export default function PortfolioPage() {
  const [isSignedIn, setIsSignedIn] = useState(true);
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [quotes, setQuotes] = useState<Quote[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        const accountResponse = await fetch(`${apiURL}/api/v1/account`, {
          credentials: "include",
          signal: controller.signal,
        });
        if (accountResponse.status === 401) {
          setIsSignedIn(false);
          return;
        }
        if (!accountResponse.ok) {
          setError("We could not load your paper account.");
          return;
        }
        setIsSignedIn(true);
        const snapshot = (await accountResponse.json()) as AccountSnapshot;
        setAccount(snapshot);

        if (snapshot.positions.length > 0) {
          const symbols = snapshot.positions.map((position) => position.symbol).join(",");
          const quotesResponse = await fetch(`${apiURL}/api/v1/quotes?symbols=${symbols}`, {
            signal: controller.signal,
          });
          if (quotesResponse.ok) {
            setQuotes((await quotesResponse.json()) as Quote[]);
          }
        }
      } catch {
        if (!controller.signal.aborted) {
          setError("TradeMind could not reach the paper-trading service.");
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false);
        }
      }
    }

    void load();
    return () => controller.abort();
  }, []);

  if (isLoading) {
    return (
      <main className={styles.page}>
        <Nav active="portfolio" />
        <p className={styles.loading}>Loading your portfolio...</p>
      </main>
    );
  }

  if (!isSignedIn) {
    return (
      <main className={styles.page}>
        <Nav active="portfolio" />
        <section className={styles.emptyState}>
          <p className={styles.eyebrow}>Paper trading</p>
          <h1>Sign in to see your portfolio.</h1>
          <a className={styles.primaryAction} href={`${apiURL}/api/v1/auth/google`}>
            Sign in with Google
          </a>
        </section>
      </main>
    );
  }

  const cashBalance = account?.cash_balance_cents ?? 0;
  const holdingsRows = buildHoldingsRows(account?.positions ?? [], quotesBySymbol(quotes));
  const marketValue = totalMarketValueCents(holdingsRows);
  const totalEquity = cashBalance + marketValue;
  const todayChange = totalTodayChangeCents(holdingsRows);
  const priorEquity = totalEquity - todayChange;
  const todayChangePct = priorEquity !== 0 ? (todayChange / priorEquity) * 100 : 0;
  const allTimeChange = allTimeReturnCents(totalEquity);
  const allTimeChangePct = allTimeReturnPct(totalEquity);

  const sortedHoldings = [...holdingsRows].sort(
    (a, b) => b.marketValueCents - a.marketValueCents,
  );
  const individualHoldings = sortedHoldings.slice(0, maxIndividualAllocationSegments);
  const foldedHoldings = sortedHoldings.slice(maxIndividualAllocationSegments);
  const foldedValueCents = foldedHoldings.reduce((sum, row) => sum + row.marketValueCents, 0);
  const allocationSegments: AllocationSegment[] = [
    { key: "cash", label: "Cash", valueCents: cashBalance, color: cashColor },
    ...individualHoldings.map((row, index) => ({
      key: row.symbol,
      label: row.symbol,
      valueCents: row.marketValueCents,
      color: allocationPalette[index % allocationPalette.length],
    })),
    ...(foldedValueCents > 0
      ? [{ key: "other", label: "Other", valueCents: foldedValueCents, color: otherColor }]
      : []),
  ];

  return (
    <main className={styles.page}>
      <Nav active="portfolio" />

      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper portfolio</p>
        <h1>Portfolio</h1>
        <p className={styles.description}>
          Allocation, position performance, and open orders for your virtual
          account.
        </p>
      </header>

      {error ? <p className={styles.error}>{error}</p> : null}

      <section className={styles.balanceGrid} aria-label="Portfolio performance">
        <article className={styles.balanceCard}>
          <p>Total equity</p>
          <strong>{currencyFromCents(totalEquity)}</strong>
          <span>Cash + holdings</span>
        </article>
        <article className={styles.balanceCard}>
          <p>Today&apos;s return</p>
          <strong className={changeClass(todayChange)}>
            {signed(todayChange)}
            {currencyFromCents(todayChange)}
          </strong>
          <span className={changeClass(todayChangePct)}>
            {signed(todayChangePct)}
            {todayChangePct.toFixed(2)}%
          </span>
        </article>
        <article className={styles.balanceCard}>
          <p>All-time return</p>
          <strong className={changeClass(allTimeChange)}>
            {signed(allTimeChange)}
            {currencyFromCents(allTimeChange)}
          </strong>
          <span className={changeClass(allTimeChangePct)}>
            {signed(allTimeChangePct)}
            {allTimeChangePct.toFixed(2)}%
          </span>
        </article>
      </section>

      <section className={styles.card}>
        <h2>Allocation</h2>
        <AllocationBar segments={allocationSegments} />
      </section>

      <section className={styles.card}>
        <h2>Positions</h2>
        <HoldingsTable rows={holdingsRows} />
      </section>

      <section className={styles.card}>
        <h2>Open orders</h2>
        <p className={styles.muted}>
          TradeMind currently supports market orders, which fill immediately —
          there are no open orders to show. Limit orders, which can stay open
          until they fill, are a planned feature.
        </p>
      </section>

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
