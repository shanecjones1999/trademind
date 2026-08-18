import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Markets",
};

export default function MarketLayout({ children }: { children: ReactNode }) {
  return children;
}
