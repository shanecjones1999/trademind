"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import styles from "./Nav.module.css";

type Profile = {
  email: string;
};

export type NavDestination = "dashboard" | "market" | "portfolio" | "history";

type NavProps = {
  active: NavDestination;
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

const destinations: { href: string; label: string; key: NavDestination }[] = [
  { href: "/dashboard", label: "Home", key: "dashboard" },
  { href: "/market", label: "Markets", key: "market" },
  { href: "/portfolio", label: "Portfolio", key: "portfolio" },
  { href: "/history", label: "History", key: "history" },
];

export default function Nav({ active }: NavProps) {
  const router = useRouter();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [isSigningOut, setIsSigningOut] = useState(false);

  useEffect(() => {
    const controller = new AbortController();

    async function loadProfile() {
      try {
        const response = await fetch(`${apiURL}/api/v1/me`, {
          credentials: "include",
          signal: controller.signal,
        });
        if (!response.ok) {
          return;
        }
        setProfile((await response.json()) as Profile);
      } catch {
        // Signed-out visitors still see the nav; the sign-in link below covers it.
      }
    }

    void loadProfile();
    return () => controller.abort();
  }, []);

  async function signOut() {
    setIsSigningOut(true);
    try {
      const response = await fetch(`${apiURL}/api/v1/auth/logout`, {
        method: "POST",
        credentials: "include",
      });
      if (response.ok) {
        router.replace("/");
        router.refresh();
      }
    } finally {
      setIsSigningOut(false);
    }
  }

  return (
    <nav className={styles.nav}>
      <Link className={styles.brand} href="/">
        TradeMind
      </Link>
      <div className={styles.links}>
        {destinations.map((destination) => (
          <Link
            className={destination.key === active ? styles.activeLink : styles.link}
            href={destination.href}
            key={destination.key}
          >
            {destination.label}
          </Link>
        ))}
      </div>
      <div className={styles.accountMenu}>
        {profile ? (
          <>
            <span className={styles.email}>{profile.email}</span>
            <button
              className={styles.signOut}
              disabled={isSigningOut}
              onClick={signOut}
              type="button"
            >
              {isSigningOut ? "Signing out..." : "Sign out"}
            </button>
          </>
        ) : (
          <a className={styles.signIn} href={`${apiURL}/api/v1/auth/google`}>
            Sign in with Google
          </a>
        )}
      </div>
    </nav>
  );
}
