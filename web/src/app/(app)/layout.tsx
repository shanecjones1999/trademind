import type { ReactNode } from "react";
import AppShell from "@/app/_components/AppShell";

export default function AppLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
