"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { apiURL } from "./api";

export type Profile = {
  subject: string;
  email: string;
  name: string;
  picture?: string;
};

type SessionStatus = "loading" | "signedOut" | "signedIn";

type SessionValue = {
  status: SessionStatus;
  profile: Profile | null;
  refresh: () => Promise<void>;
  signOut: () => Promise<boolean>;
  signOutError: string | null;
};

const SessionContext = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>("loading");
  const [profile, setProfile] = useState<Profile | null>(null);
  const [signOutError, setSignOutError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const response = await fetch(`${apiURL}/api/v1/me`, {
        credentials: "include",
      });
      if (response.status === 401 || !response.ok) {
        setProfile(null);
        setStatus("signedOut");
        return;
      }
      setProfile((await response.json()) as Profile);
      setStatus("signedIn");
    } catch {
      setProfile(null);
      setStatus("signedOut");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const signOut = useCallback(async () => {
    setSignOutError(null);
    try {
      const response = await fetch(`${apiURL}/api/v1/auth/logout`, {
        method: "POST",
        credentials: "include",
      });
      if (!response.ok) {
        setSignOutError("We could not sign you out. Please try again.");
        return false;
      }
      setProfile(null);
      setStatus("signedOut");
      return true;
    } catch {
      setSignOutError("TradeMind could not reach the account service.");
      return false;
    }
  }, []);

  const value = useMemo(
    () => ({ status, profile, refresh, signOut, signOutError }),
    [status, profile, refresh, signOut, signOutError],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession() {
  const value = useContext(SessionContext);
  if (!value) {
    throw new Error("useSession must be used within SessionProvider");
  }
  return value;
}
