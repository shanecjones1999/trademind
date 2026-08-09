"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { currency, currencyFromCents, type Quote } from "@/lib/market";
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
  positions: {
    symbol: string;
    quantity: number;
    cost_basis_cents: number;
    realized_pnl_cents: number;
  }[];
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export default function Dashboard() {
  const router = useRouter();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [account, setAccount] = useState<AccountSnapshot | null>(null);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [hasLoaded, setHasLoaded] = useState(false);
  const [accountError, setAccountError] = useState<string | null>(null);
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [signOutError, setSignOutError] = useState<string | null>(null);

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

        const [accountResponse, quoteResponse] = await Promise.all([
          fetch(`${apiURL}/api/v1/account`, {
            credentials: "include",
            signal: controller.signal,
          }),
          fetch(`${apiURL}/api/v1/quotes/AAPL`, { signal: controller.signal }),
        ]);

        if (accountResponse.ok) {
          setAccount((await accountResponse.json()) as AccountSnapshot);
        } else if (accountResponse.status === 503) {
          setAccountError(
            "Paper accounts are not configured yet. Add DATABASE_URL to the API to activate your virtual cash.",
          );
        } else {
          setAccountError("We could not load your paper account.");
        }
        if (quoteResponse.ok) {
          setQuote((await quoteResponse.json()) as Quote);
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

  async function signOut() {
    setIsSigningOut(true);
    setSignOutError(null);
    try {
      const response = await fetch(`${apiURL}/api/v1/auth/logout`, {
        method: "POST",
        credentials: "include",
      });
      if (!response.ok) {
        setSignOutError("We could not sign you out. Please try again.");
        return;
      }
      router.replace("/");
    } catch {
      setSignOutError("TradeMind could not reach the sign-out service.");
    } finally {
      setIsSigningOut(false);
    }
  }

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
  const investedCapital = positions.reduce(
    (total, position) => total + position.cost_basis_cents,
    0,
  );
  const portfolioValue = cashBalance + investedCapital;
  const changeIsPositive = quote ? quote.day_change >= 0 : true;

  return (
    <main className={styles.page}>
      <nav className={styles.nav}>
        <Link className={styles.brand} href="/">
          TradeMind
        </Link>
        <div className={styles.accountMenu}>
          <span className={styles.email}>{profile.email}</span>
          <button
            className={styles.signOut}
            disabled={isSigningOut}
            onClick={signOut}
            type="button"
          >
            {isSigningOut ? "Signing out..." : "Sign out"}
          </button>
        </div>
      </nav>

      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper portfolio</p>
        <h1>Good to see you, {firstName}.</h1>
        <p className={styles.description}>
          Practice with virtual cash and make decisions before real money is on
          the line.
        </p>
      </header>

      {signOutError ? <p className={styles.error}>{signOutError}</p> : null}

      {account ? (
        <section className={styles.balanceGrid} aria-label="Paper account summary">
          <article className={styles.primaryBalance}>
            <p>Virtual cash</p>
            <strong>{currencyFromCents(cashBalance)}</strong>
            <span>Available to invest</span>
          </article>
          <article className={styles.balanceCard}>
            <p>Portfolio value</p>
            <strong>{currencyFromCents(portfolioValue)}</strong>
            <span>
              {positions.length > 0
                ? `${positions.length} open position${positions.length === 1 ? "" : "s"}`
                : "No positions yet"}
            </span>
          </article>
          <article className={styles.balanceCard}>
            <p>Account status</p>
            <strong>{positions.length > 0 ? "Invested" : "Ready"}</strong>
            <span>
              {positions.length > 0
                ? `Cost basis ${currencyFromCents(investedCapital)}`
                : "Paper trading only"}
            </span>
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
        <article className={styles.marketCard}>
          <div className={styles.cardHeading}>
            <div>
              <p className={styles.eyebrow}>Market snapshot</p>
              <h2>Apple Inc.</h2>
            </div>
            <Link className={styles.textLink} href="/markets">
              Markets
            </Link>
          </div>
          {quote ? (
            <div className={styles.quote}>
              <div>
                <p className={styles.symbol}>{quote.symbol}</p>
                <strong>{currency(quote.price)}</strong>
              </div>
              <p
                className={
                  changeIsPositive ? styles.positiveChange : styles.negativeChange
                }
              >
                {changeIsPositive ? "+" : ""}
                {currency(quote.day_change)} ({changeIsPositive ? "+" : ""}
                {quote.day_change_pct.toFixed(2)}%)
              </p>
            </div>
          ) : (
            <p className={styles.muted}>Market data is temporarily unavailable.</p>
          )}
        </article>

        <article className={styles.nextStep}>
          <p className={styles.eyebrow}>Next step</p>
          <h2>Place your next paper trade.</h2>
          <p>
            Review quotes, pick a stock, and buy shares with your virtual cash.
          </p>
          <Link className={styles.primaryAction} href="/markets">
            Open markets
          </Link>
        </article>
      </section>

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
