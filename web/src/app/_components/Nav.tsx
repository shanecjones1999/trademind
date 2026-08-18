"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { googleAuthURL } from "@/lib/api";
import { useSession } from "@/lib/session";
import styles from "./chrome.module.css";

const destinations = [
  { href: "/dashboard", label: "Home" },
  { href: "/market", label: "Markets" },
  { href: "/portfolio", label: "Portfolio" },
  { href: "/history", label: "History" },
] as const;

export default function Nav() {
  const pathname = usePathname();
  const router = useRouter();
  const { status, profile, signOut, signOutError } = useSession();
  const nextPath = pathname && pathname !== "/" ? pathname : "/dashboard";

  async function handleSignOut() {
    if (await signOut()) {
      router.replace("/");
      router.refresh();
    }
  }

  return (
    <header className={styles.navBar}>
      <nav className={styles.nav} aria-label="TradeMind">
        <div className={styles.brandGroup}>
          <Link className={styles.brand} href="/">
            TradeMind
          </Link>
          <span className={styles.paperBadge}>Paper</span>
        </div>
        <div className={styles.links}>
          {destinations.map((destination) => {
            const isActive =
              pathname === destination.href ||
              (destination.href === "/market" && pathname === "/markets");
            return (
              <Link
                aria-current={isActive ? "page" : undefined}
                className={isActive ? styles.activeLink : styles.link}
                href={destination.href}
                key={destination.href}
              >
                {destination.label}
              </Link>
            );
          })}
        </div>
        <div className={styles.accountMenu}>
          {status === "signedIn" && profile ? (
            <>
              <span className={styles.email}>{profile.email}</span>
              <button className={styles.signOut} onClick={() => void handleSignOut()} type="button">
                Sign out
              </button>
            </>
          ) : status === "loading" ? (
            <span className={styles.email}>Checking session…</span>
          ) : (
            <a className={styles.signIn} href={googleAuthURL(nextPath)}>
              Sign in with Google
            </a>
          )}
        </div>
      </nav>
      {signOutError ? <p className={styles.signOutError}>{signOutError}</p> : null}
    </header>
  );
}
