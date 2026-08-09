export type Quote = {
  symbol: string;
  price: number;
  day_change: number;
  day_change_pct: number;
  as_of: string;
  source: string;
};

const apiURL = process.env.API_URL ?? "http://localhost:8080";

export async function getQuote(symbol: string): Promise<Quote | null> {
  try {
    const response = await fetch(`${apiURL}/api/v1/quotes/${symbol}`, {
      cache: "no-store",
    });
    if (!response.ok) {
      return null;
    }
    return (await response.json()) as Quote;
  } catch {
    return null;
  }
}

export function currency(value: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  }).format(value);
}

export function currencyFromCents(value: number) {
  return currency(value / 100);
}
