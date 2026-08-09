package paper

import (
	"errors"
	"testing"
	"time"
)

func TestPositionLedgerAppliesFIFOLotsAndRealizedProfit(t *testing.T) {
	ledger, err := NewPositionLedger("account-1")
	if err != nil {
		t.Fatalf("create position ledger: %v", err)
	}
	now := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)

	for _, fill := range []OrderFill{
		testOrderFill("buy-1", OrderSideBuy, 3, 10_000, now),
		testOrderFill("buy-2", OrderSideBuy, 2, 11_000, now.Add(time.Minute)),
	} {
		if _, err := ledger.ApplyFill(fill); err != nil {
			t.Fatalf("apply purchase: %v", err)
		}
	}
	position, err := ledger.ApplyFill(testOrderFill("sell-1", OrderSideSell, 4, 13_000, now.Add(2*time.Minute)))
	if err != nil {
		t.Fatalf("apply sale: %v", err)
	}
	if position.Quantity != 1 {
		t.Fatalf("quantity = %d, want 1", position.Quantity)
	}
	if position.CostBasisCents != 11_000 {
		t.Fatalf("cost basis = %d, want 11000", position.CostBasisCents)
	}
	if position.RealizedPnLCents != 11_000 {
		t.Fatalf("realized P&L = %d, want 11000", position.RealizedPnLCents)
	}
}

func TestPositionLedgerRejectsOversellingWithoutMutatingLots(t *testing.T) {
	ledger, err := NewPositionLedger("account-1")
	if err != nil {
		t.Fatalf("create position ledger: %v", err)
	}
	now := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	if _, err := ledger.ApplyFill(testOrderFill("buy-1", OrderSideBuy, 1, 10_000, now)); err != nil {
		t.Fatalf("apply purchase: %v", err)
	}
	if _, err := ledger.ApplyFill(testOrderFill("sell-1", OrderSideSell, 2, 11_000, now.Add(time.Minute))); !errors.Is(err, ErrInsufficientPosition) {
		t.Fatalf("oversell error = %v, want ErrInsufficientPosition", err)
	}

	positions, err := ledger.Positions()
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 1 || positions[0].Quantity != 1 || positions[0].CostBasisCents != 10_000 {
		t.Fatalf("positions after failed sale = %#v", positions)
	}
}

func TestPositionLedgerOmitsClosedPositions(t *testing.T) {
	ledger, err := NewPositionLedger("account-1")
	if err != nil {
		t.Fatalf("create position ledger: %v", err)
	}
	now := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	if _, err := ledger.ApplyFill(testOrderFill("buy-1", OrderSideBuy, 1, 10_000, now)); err != nil {
		t.Fatalf("apply purchase: %v", err)
	}
	if _, err := ledger.ApplyFill(testOrderFill("sell-1", OrderSideSell, 1, 11_000, now.Add(time.Minute))); err != nil {
		t.Fatalf("apply sale: %v", err)
	}

	positions, err := ledger.Positions()
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 0 {
		t.Fatalf("positions = %#v, want no open positions", positions)
	}
}

func testOrderFill(id string, side OrderSide, quantity int64, price Money, occurredAt time.Time) OrderFill {
	return OrderFill{
		Order: Order{
			ID:        id,
			AccountID: "account-1",
			Symbol:    "AAPL",
			Side:      side,
			Type:      OrderTypeMarket,
			Quantity:  quantity,
			Status:    OrderStatusFilled,
		},
		Execution: Execution{
			ID:         "execution:" + id,
			OrderID:    id,
			Symbol:     "AAPL",
			Quantity:   quantity,
			PriceCents: price,
			OccurredAt: occurredAt,
		},
	}
}
