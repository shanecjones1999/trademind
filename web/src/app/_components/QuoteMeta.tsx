import { quoteMetaLabel, usCashHoursLabel, type USHoursStatus } from "@/lib/quote-meta";
import type { Quote } from "@/lib/market";
import styles from "./chrome.module.css";

type QuoteMetaProps = {
  quote: Pick<Quote, "as_of" | "source">;
  hours?: USHoursStatus;
};

export default function QuoteMeta({ quote, hours }: QuoteMetaProps) {
  return (
    <p className={styles.quoteMeta}>
      {quoteMetaLabel(quote)}
      {hours ? <span className={styles.hours}> · {usCashHoursLabel(hours)}</span> : null}
    </p>
  );
}
