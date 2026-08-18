"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
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

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positiveChange : styles.negativeChange;
}

export default function Dashboard() {
  const { status, profile } = useSession();
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [quotes, setQuotes] = useState<Quote[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [accountError, setAccountError] = useState<string | null>(null);

  const loadAccount = useCallback(async () => {
    if (status !== "signedIn") {
      return;
    }
    setIsLoading(true);
    setAccountError(null);
    try {
      const accountResponse = await fetch(`${apiURL}/api/v1/account`, {
        credentials: "include",
      });
      if (!accountResponse.ok) {
        setAccountError(
          accountResponse.status === 503
            ? "Paper accounts are not available right now. Please try again shortly."
            : "We could not load your paper account.",
        );
        return;
      }
      const snapshot = (await accountResponse.json()) as AccountSnapshot;
      setAccount(snapshot);
      if (snapshot.positions.length === 0) {
        setQuotes([]);
        return;
      }
      const symbols = snapshot.positions.map((position) => position.symbol).join(",");
      const quoteResponse = await fetch(`${apiURL}/api/v1/quotes?symbols=${symbols}`);
      if (quoteResponse.ok) {
        setQuotes((await quoteResponse.json()) as Quote[]);
      }
    } catch {
      setAccountError("TradeMind could not reach the account service.");
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
    void loadAccount();
  }, [loadAccount, status]);

  if (status === "loading" || (status === "signedIn" && isLoading && !account)) {
    return (
      <main>
        <header className={styles.intro}>
          <p className={styles.eyebrow}>Your paper portfolio</p>
          <h1>Home</h1>
        </header>
        <PageSkeleton />
      </main>
    );
  }

  if (status === "signedOut" || !profile) {
    return <SignedOutGate next="/dashboard" title="Sign in to start building your virtual portfolio." />;
  }

  const firstName = profile.name.split(" ")[0] || profile.email;
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

  return (
    <main>
      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper portfolio</p>
        <h1>Good to see you, {firstName}.</h1>
        <p className={styles.description}>
          Practice with virtual cash and make decisions before real money is on the line.
        </p>
      </header>

      {accountError ? <ErrorBanner message={accountError} onRetry={() => void loadAccount()} /> : null}

      {account ? (
        <section className={styles.balanceGrid} aria-label="Paper account summary">
          <article className={styles.primaryBalance}>
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
          <article className={styles.balanceCard}>
            <p>Buying power</p>
            <strong>{currencyFromCents(cashBalance)}</strong>
            <span>Available to invest</span>
          </article>
        </section>
      ) : null}

      <section className={styles.workspace}>
        <article className={styles.holdingsCard}>
          <div className={styles.cardHeading}>
            <div>
              <p className={styles.eyebrow}>Holdings</p>
              <h2>Your open positions.</h2>
            </div>
            <Link className={styles.textLink} href="/market">
              Markets
            </Link>
          </div>
          {freshness ? (
            <div className={styles.freshness}>
              <QuoteMeta hours={usCashHoursStatus()} quote={freshness} />
            </div>
          ) : null}
          <HoldingsTable rows={holdingsRows} />
        </article>

        {holdingsRows.length === 0 ? (
          <article className={styles.nextStep}>
            <p className={styles.eyebrow}>Next step</p>
            <h2>Place your next paper trade.</h2>
            <p>Review quotes, pick a stock, and buy shares with your virtual cash.</p>
            <Link className={styles.primaryAction} href="/market">
              Open markets
            </Link>
          </article>
        ) : null}
      </section>
    </main>
  );
}
