"use client";

import Link from "next/link";
import { Suspense, useCallback, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import ErrorBanner from "@/app/_components/ErrorBanner";
import PageSkeleton from "@/app/_components/PageSkeleton";
import SignedOutGate from "@/app/_components/SignedOutGate";
import TradeHistoryTable, {
  type TradeHistoryOrder,
} from "@/app/_components/TradeHistoryTable";
import { apiURL } from "@/lib/api";
import { useSession } from "@/lib/session";
import styles from "./page.module.css";

type OrdersResponse = {
  orders: TradeHistoryOrder[];
  total: number;
  limit: number;
  offset: number;
};

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
        <main>
          <header className={styles.intro}>
            <p className={styles.eyebrow}>Your paper trades</p>
            <h1>History</h1>
          </header>
          <PageSkeleton cards={1} />
        </main>
      }
    >
      <HistoryContent />
    </Suspense>
  );
}

function HistoryContent() {
  const { status } = useSession();
  const router = useRouter();
  const searchParams = useSearchParams();
  const page = pageFromSearch(searchParams.get("page"));
  const [orders, setOrders] = useState<TradeHistoryOrder[]>([]);
  const [total, setTotal] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (status !== "signedIn") {
      return;
    }
    setError(null);
    setIsLoading(true);
    try {
      const offset = (page - 1) * pageSize;
      const response = await fetch(`${apiURL}/api/v1/orders?limit=${pageSize}&offset=${offset}`, {
        credentials: "include",
      });
      if (!response.ok) {
        setError("We could not load your trade history.");
        return;
      }
      const payload = (await response.json()) as OrdersResponse;
      setOrders(payload.orders ?? []);
      setTotal(payload.total ?? 0);
    } catch {
      setError("TradeMind could not reach the paper-trading service.");
    } finally {
      setIsLoading(false);
    }
  }, [page, status]);

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

  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  useEffect(() => {
    if (isLoading || total === 0 || page <= pageCount) {
      return;
    }
    router.replace(historyHref(pageCount));
  }, [isLoading, page, pageCount, router, total]);

  if (status === "loading" || (status === "signedIn" && isLoading && orders.length === 0 && !error)) {
    return (
      <main>
        <header className={styles.intro}>
          <p className={styles.eyebrow}>Your paper trades</p>
          <h1>History</h1>
        </header>
        <PageSkeleton cards={1} />
      </main>
    );
  }

  if (status === "signedOut") {
    return <SignedOutGate next="/history" title="Sign in to see your trade history." />;
  }

  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);
  const hasPrevious = page > 1;
  const hasNext = page < pageCount && total > 0;

  return (
    <main>
      <header className={styles.intro}>
        <p className={styles.eyebrow}>Your paper trades</p>
        <h1>History</h1>
        <p className={styles.description}>
          Filled market orders for this account, newest first. Realized P&amp;L is shown on sells.
        </p>
      </header>

      {error ? <ErrorBanner message={error} onRetry={() => void load()} /> : null}

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
    </main>
  );
}
