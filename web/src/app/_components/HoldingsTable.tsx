import Link from "next/link";
import { currencyFromCents } from "@/lib/market";
import type { HoldingRow } from "@/lib/portfolio";
import styles from "./HoldingsTable.module.css";

type HoldingsTableProps = {
  rows: HoldingRow[];
};

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positive : styles.negative;
}

export default function HoldingsTable({ rows }: HoldingsTableProps) {
  if (rows.length === 0) {
    return <p className={styles.empty}>No positions yet.</p>;
  }

  return (
    <div className={styles.tableWrap}>
      <table className={styles.table}>
        <thead>
          <tr>
            <th scope="col">Symbol</th>
            <th scope="col">Shares</th>
            <th scope="col">Avg cost</th>
            <th scope="col">Price</th>
            <th scope="col">Market value</th>
            <th scope="col">Today</th>
            <th scope="col">Unrealized P&amp;L</th>
            <th scope="col">Realized P&amp;L</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.symbol}>
              <th scope="row">
                <Link href={`/market?symbol=${encodeURIComponent(row.symbol)}`}>{row.symbol}</Link>
              </th>
              <td>{row.quantity}</td>
              <td>{currencyFromCents(row.avgCostCents)}</td>
              <td>{row.priceCents !== null ? currencyFromCents(row.priceCents) : "Unavailable"}</td>
              <td>{currencyFromCents(row.marketValueCents)}</td>
              <td className={row.todayChangeCents !== null ? changeClass(row.todayChangeCents) : undefined}>
                {row.todayChangeCents !== null
                  ? `${signed(row.todayChangeCents)}${currencyFromCents(row.todayChangeCents)}`
                  : "Unavailable"}
              </td>
              <td className={row.unrealizedPnLCents !== null ? changeClass(row.unrealizedPnLCents) : undefined}>
                {row.unrealizedPnLCents !== null && row.unrealizedPnLPct !== null
                  ? `${signed(row.unrealizedPnLCents)}${currencyFromCents(row.unrealizedPnLCents)} (${signed(
                      row.unrealizedPnLPct,
                    )}${row.unrealizedPnLPct.toFixed(2)}%)`
                  : "Unavailable"}
              </td>
              <td className={row.realizedPnLCents !== 0 ? changeClass(row.realizedPnLCents) : undefined}>
                {signed(row.realizedPnLCents)}
                {currencyFromCents(row.realizedPnLCents)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
