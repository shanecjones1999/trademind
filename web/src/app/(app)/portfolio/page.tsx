"use client";

import { useCallback, useEffect, useState } from "react";
import AllocationBar, {
  allocationPalette,
  cashColor,
  otherColor,
  type AllocationSegment,
} from "@/app/_components/AllocationBar";
import ErrorBanner from "@/app/_components/ErrorBanner";
import HoldingsTable from "@/app/_components/HoldingsTable";
import PageSkeleton from "@/app/_components/PageSkeleton";
import QuoteMeta from "@/app/_components/QuoteMeta";
import SignedOutGate from "@/app/_components/SignedOutGate";
import { apiURL } from "@/lib/api";
import { currencyFromCents, type Quote } from "@/lib/market";
import { sharedQuoteMeta, usCashHoursStatus } from "@/lib/quote-meta";
import { useSession } from "@/lib/session";
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

const maxIndividualAllocationSegments = 7;

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positiveChange : styles.negativeChange;
}

export default function PortfolioPage() {
  const { status } = useSession();
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [quotes, setQuotes] = useState<Quote[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (status !== "signedIn") {
      return;
    }
    setIsLoading(true);
    setError(null);
    try {
      const accountResponse = await fetch(`${apiURL}/api/v1/account`, {
        credentials: "include",
      });
      if (!accountResponse.ok) {
        setError("We could not load your paper account.");
        return;
      }
      const snapshot = (await accountResponse.json()) as AccountSnapshot;
      setAccount(snapshot);
      if (snapshot.positions.length === 0) {
        setQuotes([]);
        return;
      }
      const symbols = snapshot.positions.map((position) => position.symbol).join(",");
      const quotesResponse = await fetch(`${apiURL}/api/v1/quotes?symbols=${symbols}`);
      if (quotesResponse.ok) {
        setQuotes((await quotesResponse.json()) as Quote[]);
      }
    } catch {
      setError("TradeMind could not reach the paper-trading service.");
    } finally {
      setIsLoading(false);
    }
  }, [status]);

  useEffect(() => {
    if (status === "loading") {
      return;
    }
    if (status === "signedOut") {
      setIsLoading(false);
      return;
    }
    void load();
  }, [load, status]);

  if (status === "loading" || (status === "signedIn" && isLoading && !account)) {
    return (
      <main>
        <header className={styles.intro}>
          <p className={styles.eyebrow}>Your paper portfolio</p>
          <h1>Portfolio</h1>
        </header>
        <PageSkeleton cards={3} />
      </main>
    );
  }

  if (status === "signedOut") {
    return <SignedOutGate next="/portfolio" title="Sign in to see your portfolio." />;
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
  const freshness = sharedQuoteMeta(quotes);

  const sortedHoldings = [...holdingsRows].sort((a, b) => b.marketValueCents - a.marketValueCents);
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
    <main>
      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper portfolio</p>
        <h1>Portfolio</h1>
        <p className={styles.description}>
          Allocation and position performance for your virtual account.
        </p>
      </header>

      {error ? <ErrorBanner message={error} onRetry={() => void load()} /> : null}

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
        {freshness ? (
          <div className={styles.freshness}>
            <QuoteMeta hours={usCashHoursStatus()} quote={freshness} />
          </div>
        ) : null}
        <HoldingsTable rows={holdingsRows} />
      </section>
    </main>
  );
}
