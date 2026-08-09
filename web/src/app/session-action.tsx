"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

type SessionActionProps = {
  className: string;
  signedInLabel: string;
  signedOutLabel: string;
};

const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export default function SessionAction({
  className,
  signedInLabel,
  signedOutLabel,
}: SessionActionProps) {
  const [isSignedIn, setIsSignedIn] = useState<boolean | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    async function checkSession() {
      try {
        const response = await fetch(`${apiURL}/api/v1/me`, {
          credentials: "include",
        });
        if (response.ok) {
          setIsSignedIn(true);
          return;
        }
        if (response.status === 401) {
          setIsSignedIn(false);
          return;
        }
        setError(true);
      } catch {
        setError(true);
      }
    }

    void checkSession();
  }, []);

  if (isSignedIn === null) {
    if (error) {
      return <span className={className}>Session unavailable</span>;
    }
    return <span className={className}>Loading...</span>;
  }

  if (isSignedIn) {
    return (
      <Link className={className} href="/dashboard">
        {signedInLabel}
      </Link>
    );
  }

  return (
    <a className={className} href={`${apiURL}/api/v1/auth/google`}>
      {signedOutLabel}
    </a>
  );
}
