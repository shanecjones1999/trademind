"use client";

import Link from "next/link";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import Nav from "@/app/_components/Nav";
import TradeHistoryTable, {
  type TradeHistoryOrder,
} from "@/app/_components/TradeHistoryTable";
import styles from "./page.module.css";

type OrdersResponse = {
  orders: TradeHistoryOrder[];
  total: number;
  limit: number;
  offset: number;
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const pageSize = 25;

function historyHref(page: number) {
  if (page <= 1) {
    return "/history";
  }
  return `/history?page=${page}`;
}

function pageFromSearch(rawPage: string | null) {
  const page = Number.parseInt(rawPage ?? "1", 10);
  if (!Number.isFinite(page) || page < 1) {
    return 1;
  }
  return page;
}

export default function HistoryPage() {
  return (
    <Suspense
      fallback={
        <main className={styles.page}>
          <Nav active="history" />
          <p className={styles.loading}>Loading your trade history...</p>
        </main>
      }
    >
      <HistoryContent />
    </Suspense>
  );
}

function HistoryContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const page = pageFromSearch(searchParams.get("page"));
  const [isSignedIn, setIsSignedIn] = useState(true);
  const [orders, setOrders] = useState<TradeHistoryOrder[]>([]);
  const [total, setTotal] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    async function load() {
      setError(null);
      try {
        const offset = (page - 1) * pageSize;
        const response = await fetch(
          `${apiURL}/api/v1/orders?limit=${pageSize}&offset=${offset}`,
          {
            credentials: "include",
            signal: controller.signal,
          },
        );
        if (response.status === 401) {
          setIsSignedIn(false);
          return;
        }
        if (!response.ok) {
          setError("We could not load your trade history.");
          return;
        }
        setIsSignedIn(true);
        const payload = (await response.json()) as OrdersResponse;
        setOrders(payload.orders ?? []);
        setTotal(payload.total ?? 0);
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
  }, [page]);

  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    if (isLoading || total === 0 || page <= pageCount) {
      return;
    }
    router.replace(historyHref(pageCount));
  }, [isLoading, page, pageCount, router, total]);

  if (isLoading) {
    return (
      <main className={styles.page}>
        <Nav active="history" />
        <p className={styles.loading}>Loading your trade history...</p>
      </main>
    );
  }

  if (!isSignedIn) {
    return (
      <main className={styles.page}>
        <Nav active="history" />
        <section className={styles.emptyState}>
          <p className={styles.eyebrow}>Paper trading</p>
          <h1>Sign in to see your trade history.</h1>
          <a className={styles.primaryAction} href={`${apiURL}/api/v1/auth/google`}>
            Sign in with Google
          </a>
        </section>
      </main>
    );
  }

  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);
  const hasPrevious = page > 1;
  const hasNext = page < pageCount && total > 0;

  return (
    <main className={styles.page}>
      <Nav active="history" />

      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper trades</p>
        <h1>History</h1>
        <p className={styles.description}>
          Filled market orders for this account, newest first. Realized P&amp;L
          is shown on sells.
        </p>
      </header>

      {error ? <p className={styles.error}>{error}</p> : null}

      <section className={styles.card}>
        <h2>Trade history</h2>
        <TradeHistoryTable orders={orders} />
        {total > 0 ? (
          <nav className={styles.pagination} aria-label="Trade history pages">
            <p className={styles.pageStatus}>
              Showing {rangeStart}–{rangeEnd} of {total}
            </p>
            <div className={styles.pageActions}>
              {hasPrevious ? (
                <Link className={styles.pageButton} href={historyHref(page - 1)}>
                  Previous
                </Link>
              ) : (
                <span className={styles.pageButtonDisabled}>Previous</span>
              )}
              {hasNext ? (
                <Link className={styles.pageButton} href={historyHref(page + 1)}>
                  Next
                </Link>
              ) : (
                <span className={styles.pageButtonDisabled}>Next</span>
              )}
            </div>
          </nav>
        ) : null}
      </section>

      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice.
      </p>
    </main>
  );
}
