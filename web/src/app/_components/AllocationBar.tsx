import { currencyFromCents } from "@/lib/market";
import styles from "./AllocationBar.module.css";

export type AllocationSegment = {
  key: string;
  label: string;
  valueCents: number;
  color: string;
};

type AllocationBarProps = {
  segments: AllocationSegment[];
};

// Reference categorical order from the dataviz skill's validated default
// palette (references/palette.md) — fixed order, never cycled or reordered.
export const allocationPalette = [
  "#2a78d6", // blue
  "#eb6834", // orange
  "#1baf7a", // aqua
  "#eda100", // yellow
  "#e87ba4", // magenta
  "#008300", // green
  "#4a3aa7", // violet
];

// Cash is not a categorical series — it is the uninvested remainder — so it
// gets a fixed neutral rather than a slot from the categorical order.
export const cashColor = "#c8d7ce";
export const otherColor = "#75867d";

export default function AllocationBar({ segments }: AllocationBarProps) {
  const total = segments.reduce((sum, segment) => sum + segment.valueCents, 0);
  if (total <= 0) {
    return <p className={styles.empty}>Nothing to allocate yet.</p>;
  }

  return (
    <div>
      <div className={styles.bar} role="img" aria-label="Portfolio allocation by holding">
        {segments.map((segment) => (
          <span
            className={styles.segment}
            key={segment.key}
            style={{ background: segment.color, flexGrow: segment.valueCents }}
            title={`${segment.label}: ${((segment.valueCents / total) * 100).toFixed(1)}%`}
          />
        ))}
      </div>
      <ul className={styles.legend}>
        {segments.map((segment) => (
          <li key={segment.key}>
            <span className={styles.swatch} style={{ background: segment.color }} />
            <span className={styles.legendLabel}>{segment.label}</span>
            <span className={styles.legendValue}>
              {currencyFromCents(segment.valueCents)} ·{" "}
              {((segment.valueCents / total) * 100).toFixed(1)}%
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}
