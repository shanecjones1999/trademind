import type { ReactNode } from "react";
import Nav from "./Nav";
import styles from "./chrome.module.css";

export default function AppShell({ children }: { children: ReactNode }) {
  return (
    <div className={styles.shell}>
      <Nav />
      <div className={styles.body}>{children}</div>
      <p className={styles.disclosure}>
        TradeMind is a simulated trading experience, not investment advice. Quotes
        shown are delayed previous-close prices unless marked otherwise.
      </p>
    </div>
  );
}
