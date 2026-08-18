"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { googleAuthURL } from "@/lib/api";
import styles from "./page.module.css";

export default function AuthBanner() {
  return (
    <Suspense fallback={null}>
      <AuthBannerMessage />
    </Suspense>
  );
}

function AuthBannerMessage() {
  const searchParams = useSearchParams();
  if (searchParams.get("auth") !== "error") {
    return null;
  }
  return (
    <p className={styles.authBanner} role="alert">
      Google sign-in did not complete.{" "}
      <a href={googleAuthURL("/dashboard")}>Try again</a>.
    </p>
  );
}
