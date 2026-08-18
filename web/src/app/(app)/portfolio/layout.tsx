import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Portfolio",
};

export default function PortfolioLayout({ children }: { children: ReactNode }) {
  return children;
}
