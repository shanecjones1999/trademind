import Link from "next/link";
import styles from "./error.module.css";

export default function NotFound() {
  return (
    <main className={styles.page}>
      <p className={styles.eyebrow}>Page not found</p>
      <h1>That screen is not in TradeMind.</h1>
      <p className={styles.copy}>Check the address, or head back to your paper portfolio.</p>
      <Link className={styles.primary} href="/dashboard">
        Go to Home
      </Link>
    </main>
  );
}
