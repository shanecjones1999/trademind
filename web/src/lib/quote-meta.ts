import type { Quote } from "./market";

export function formatQuoteDate(asOf: string) {
  const date = new Date(asOf);
  if (Number.isNaN(date.getTime())) {
    return "Unknown date";
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeZone: "UTC",
  }).format(date);
}

export function quoteMetaLabel(quote: Pick<Quote, "as_of" | "source">) {
  return `Previous close · ${formatQuoteDate(quote.as_of)} · ${quote.source}`;
}

export type USHoursStatus = "open" | "pre" | "after" | "closed";

export function usCashHoursStatus(now = new Date()): USHoursStatus {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "America/New_York",
    weekday: "short",
    hour: "numeric",
    minute: "numeric",
    hourCycle: "h23",
  }).formatToParts(now);
  const weekday = parts.find((part) => part.type === "weekday")?.value;
  const hour = Number(parts.find((part) => part.type === "hour")?.value ?? "0");
  const minute = Number(parts.find((part) => part.type === "minute")?.value ?? "0");
  if (weekday === "Sat" || weekday === "Sun") {
    return "closed";
  }
  const minutes = hour * 60 + minute;
  if (minutes < 9 * 60 + 30) {
    return "pre";
  }
  if (minutes < 16 * 60) {
    return "open";
  }
  return "after";
}

export function usCashHoursLabel(status: USHoursStatus = usCashHoursStatus()) {
  switch (status) {
    case "open":
      return "US hours · Open";
    case "pre":
      return "US hours · Pre-market";
    case "after":
      return "US hours · After hours";
    default:
      return "US hours · Closed";
  }
}

export function sharedQuoteMeta(quotes: Quote[]) {
  if (quotes.length === 0) {
    return null;
  }
  const first = quotes[0];
  if (!quotes.every((quote) => quote.as_of === first.as_of && quote.source === first.source)) {
    return null;
  }
  return first;
}
