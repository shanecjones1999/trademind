import { googleAuthURL } from "@/lib/api";
import styles from "./chrome.module.css";

type SignedOutGateProps = {
  next: string;
  title: string;
};

export default function SignedOutGate({ next, title }: SignedOutGateProps) {
  return (
    <section className={styles.emptyState}>
      <p className={styles.eyebrow}>Paper trading</p>
      <h1>{title}</h1>
      <a className={styles.primaryAction} href={googleAuthURL(next)}>
        Sign in with Google
      </a>
    </section>
  );
}
