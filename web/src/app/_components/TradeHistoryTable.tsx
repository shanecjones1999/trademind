import Link from "next/link";
import { currencyFromCents } from "@/lib/market";
import styles from "./TradeHistoryTable.module.css";

export type TradeHistoryOrder = {
  id: string;
  symbol: string;
  side: "buy" | "sell";
  quantity: number;
  submitted_at: string;
  execution: {
    price_cents: number;
    occurred_at: string;
    gross_cents: number;
    realized_pnl_cents: number | null;
  };
};

type TradeHistoryTableProps = {
  orders: TradeHistoryOrder[];
};

function signed(value: number) {
  return value >= 0 ? "+" : "";
}

function changeClass(value: number) {
  return value >= 0 ? styles.positive : styles.negative;
}

function formatTradeTime(value: string) {
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export default function TradeHistoryTable({ orders }: TradeHistoryTableProps) {
  if (orders.length === 0) {
    return (
      <p className={styles.empty}>
        No trades yet. Place a market order from{" "}
        <Link href="/market">Markets</Link>.
      </p>
    );
  }

  return (
    <div className={styles.tableWrap}>
      <table className={styles.table}>
        <thead>
          <tr>
            <th scope="col">Time</th>
            <th scope="col">Side</th>
            <th scope="col">Symbol</th>
            <th scope="col">Qty</th>
            <th scope="col">Fill price</th>
            <th scope="col">Total</th>
            <th scope="col">Realized P&amp;L</th>
          </tr>
        </thead>
        <tbody>
          {orders.map((order) => {
            const realized = order.execution.realized_pnl_cents;
            return (
              <tr key={order.id}>
                <th scope="row">{formatTradeTime(order.execution.occurred_at)}</th>
                <td className={order.side === "buy" ? styles.buy : styles.sell}>
                  {order.side === "buy" ? "Buy" : "Sell"}
                </td>
                <td className={styles.symbol}>{order.symbol}</td>
                <td>{order.quantity}</td>
                <td>{currencyFromCents(order.execution.price_cents)}</td>
                <td className={changeClass(order.execution.gross_cents)}>
                  {signed(order.execution.gross_cents)}
                  {currencyFromCents(order.execution.gross_cents)}
                </td>
                <td className={realized !== null ? changeClass(realized) : undefined}>
                  {realized !== null
                    ? `${signed(realized)}${currencyFromCents(realized)}`
                    : "—"}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
