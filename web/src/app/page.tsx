import Link from "next/link";
import styles from "./page.module.css";
import SessionAction from "./session-action";
import { currency, getQuote } from "@/lib/market";

export default async function Home() {
  const quote = await getQuote("AAPL");
  const changeIsPositive = quote ? quote.day_change >= 0 : true;
  return (
    <main className={styles.page}>
      <nav className={styles.nav}>
        <Link className={styles.brand} href="/">
          TradeMind
        </Link>
        <SessionAction
          className={styles.signIn}
          signedInLabel="Your dashboard"
          signedOutLabel="Sign in with Google"
        />
      </nav>

      <section className={styles.hero}>
        <p className={styles.eyebrow}>Paper trading, built for practice</p>
        <h1>Build confidence before you invest real money.</h1>
        <p className={styles.description}>
          Follow real market data, create a virtual portfolio, and learn by
          making simulated trades.
        </p>
        <SessionAction
          className={styles.primaryAction}
          signedInLabel="View your dashboard"
          signedOutLabel="Start paper trading"
        />
      </section>

      <section className={styles.quoteSection} aria-labelledby="market-title">
        <div>
          <p className={styles.eyebrow}>Market snapshot</p>
          <h2 id="market-title">See the market in motion.</h2>
        </div>
        {quote ? (
          <article className={styles.quoteCard}>
            <div className={styles.quoteHeader}>
              <strong>{quote.symbol}</strong>
              <span>Previous close</span>
            </div>
            <p className={styles.price}>{currency(quote.price)}</p>
            <p
              className={
                changeIsPositive ? styles.positiveChange : styles.negativeChange
              }
            >
              {changeIsPositive ? "+" : ""}
              {currency(quote.day_change)} ({changeIsPositive ? "+" : ""}
              {quote.day_change_pct.toFixed(2)}%)
            </p>
            <p className={styles.quoteMeta}>
              {quote.source} data from{" "}
              {new Intl.DateTimeFormat("en-US", {
                dateStyle: "medium",
                timeZone: "UTC",
              }).format(new Date(quote.as_of))}
            </p>
          </article>
        ) : (
          <article className={styles.quoteCard}>
            <p className={styles.unavailable}>Market data is unavailable.</p>
            <p className={styles.quoteMeta}>
              Start the Go API and add your Massive key to load the daily quote.
            </p>
          </article>
        )}
      </section>

      <section className={styles.features} aria-label="TradeMind features">
        <article>
          <h2>Virtual cash</h2>
          <p>Practice your strategy without putting real money at risk.</p>
        </article>
        <article>
          <h2>Clear performance</h2>
          <p>Track holdings, returns, and decisions in one focused portfolio.</p>
        </article>
        <article>
          <h2>Learn as you go</h2>
          <p>Make simulated moves now and build investing habits over time.</p>
        </article>
      </section>

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
