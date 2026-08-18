"use client";

import { useRouter } from "next/navigation";
import styles from "./error.module.css";

export default function ErrorPage({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const router = useRouter();
  return (
    <main className={styles.page}>
      <p className={styles.eyebrow}>Something went wrong</p>
      <h1>TradeMind hit a snag.</h1>
      <p className={styles.copy}>
        {error.message || "Please try again. Your paper account is unchanged."}
      </p>
      <div className={styles.actions}>
        <button className={styles.primary} onClick={reset} type="button">
          Try again
        </button>
        <button className={styles.secondary} onClick={() => router.push("/dashboard")} type="button">
          Back to Home
        </button>
      </div>
    </main>
  );
}
