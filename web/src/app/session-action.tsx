"use client";

import Link from "next/link";
import { googleAuthURL } from "@/lib/api";
import { useSession } from "@/lib/session";

type SessionActionProps = {
  className: string;
  signedInLabel: string;
  signedOutLabel: string;
};

export default function SessionAction({
  className,
  signedInLabel,
  signedOutLabel,
}: SessionActionProps) {
  const { status } = useSession();

  if (status === "loading") {
    return <span className={className}>Checking session…</span>;
  }

  if (status === "signedIn") {
    return (
      <Link className={className} href="/dashboard">
        {signedInLabel}
      </Link>
    );
  }

  return (
    <a className={className} href={googleAuthURL("/dashboard")}>
      {signedOutLabel}
    </a>
  );
}
