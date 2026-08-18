"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import Nav from "@/app/_components/Nav";
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

type Profile = {
  subject: string;
  email: string;
  name: string;
  picture?: string;
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

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positiveChange : styles.negativeChange;
}

export default function Dashboard() {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [quotes, setQuotes] = useState<Quote[]>([]);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [accountError, setAccountError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    async function loadProfile() {
      try {
        const profileResponse = await fetch(`${apiURL}/api/v1/me`, {
          credentials: "include",
          signal: controller.signal,
        });
        if (profileResponse.status === 401) {
          return;
        }
        if (!profileResponse.ok) {
          setAccountError("Your account details are temporarily unavailable.");
          return;
        }
        setProfile((await profileResponse.json()) as Profile);

        const accountResponse = await fetch(`${apiURL}/api/v1/account`, {
          credentials: "include",
          signal: controller.signal,
        });

        if (!accountResponse.ok) {
          setAccountError(
            accountResponse.status === 503
              ? "Paper accounts are not configured yet. Add DATABASE_URL to the API to activate your virtual cash."
              : "We could not load your paper account.",
          );
          return;
        }

        const snapshot = (await accountResponse.json()) as AccountSnapshot;
        setAccount(snapshot);

        if (snapshot.positions.length > 0) {
          const symbols = snapshot.positions.map((position) => position.symbol).join(",");
          const quoteResponse = await fetch(`${apiURL}/api/v1/quotes?symbols=${symbols}`, {
            signal: controller.signal,
          });
          if (quoteResponse.ok) {
            setQuotes((await quoteResponse.json()) as Quote[]);
          }
        }
      } catch {
        if (!controller.signal.aborted) {
          setAccountError(
            "TradeMind could not reach the account service. Check that the API is running.",
          );
        }
      } finally {
        if (!controller.signal.aborted) {
          setHasLoaded(true);
        }
      }
    }

    void loadProfile();
    return () => controller.abort();
  }, []);

  if (!hasLoaded) {
    return (
      <main className={styles.page}>
        <p className={styles.loading}>Loading your TradeMind account...</p>
      </main>
    );
  }

  if (!profile) {
    return (
      <main className={styles.page}>
        <section className={styles.card}>
          <p className={styles.eyebrow}>Paper trading</p>
          <h1>Sign in to start building your virtual portfolio.</h1>
          <a className={styles.primaryAction} href={`${apiURL}/api/v1/auth/google`}>
            Sign in with Google
          </a>
          <Link className={styles.secondaryAction} href="/">
            Back to market snapshot
          </Link>
        </section>
      </main>
    );
  }

  const firstName = profile.name.split(" ")[0] || profile.email;
  const cashBalance = account?.cash_balance_cents ?? 0;
  const positions = account?.positions ?? [];
  const holdingsRows = buildHoldingsRows(positions, quotesBySymbol(quotes));
  const marketValue = totalMarketValueCents(holdingsRows);
  const totalEquity = cashBalance + marketValue;
  const todayChange = totalTodayChangeCents(holdingsRows);
  const priorEquity = totalEquity - todayChange;
  const todayChangePct = priorEquity !== 0 ? (todayChange / priorEquity) * 100 : 0;
  const allTimeChange = allTimeReturnCents(totalEquity);
  const allTimeChangePct = allTimeReturnPct(totalEquity);

  return (
    <main className={styles.page}>
      <Nav active="dashboard" />

      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper portfolio</p>
        <h1>Good to see you, {firstName}.</h1>
        <p className={styles.description}>
          Practice with virtual cash and make decisions before real money is on
          the line.
        </p>
      </header>

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
      ) : (
        <section className={styles.accountUnavailable}>
          <p className={styles.eyebrow}>Paper account</p>
          <h2>Finish setting up your virtual portfolio.</h2>
          <p>{accountError ?? "Your virtual cash balance is loading."}</p>
        </section>
      )}

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
          <HoldingsTable rows={holdingsRows} />
        </article>

        {holdingsRows.length === 0 ? (
          <article className={styles.nextStep}>
            <p className={styles.eyebrow}>Next step</p>
            <h2>Place your next paper trade.</h2>
            <p>
              Review quotes, pick a stock, and buy shares with your virtual cash.
            </p>
            <Link className={styles.primaryAction} href="/market">
              Open markets
            </Link>
          </article>
        ) : null}
      </section>

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
