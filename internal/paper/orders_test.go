package paper

import (
	"errors"
	"testing"
	"time"
)

func TestFillOrderUsesAskForBuysAndPostsBalancedCash(t *testing.T) {
	now := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	order, err := NewOrder(OrderRequest{
		ID:             "order-1",
		IdempotencyKey: "client-request-1",
		AccountID:      "paper-account-1",
		Symbol:         "aapl",
		Side:           OrderSideBuy,
		Type:           OrderTypeMarket,
		Quantity:       3,
	}, now)
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	fill, err := FillOrder(
		order,
		ExecutableQuote{
			BidCents: 19_990,
			AskCents: 20_000,
			AsOf:     now.Add(-5 * time.Second),
			Source:   "licensed feed",
		},
		FillPolicy{MaximumQuoteAge: 30 * time.Second, SlippageBps: 100},
		100_000,
		0,
		now,
	)
	if err != nil {
		t.Fatalf("fill order: %v", err)
	}
	if fill.Order.Status != OrderStatusFilled {
		t.Fatalf("status = %q, want %q", fill.Order.Status, OrderStatusFilled)
	}
	if fill.Execution.PriceCents != 20_200 {
		t.Fatalf("price = %d, want 20200", fill.Execution.PriceCents)
	}
	if fill.CashTransaction.Postings[0].Amount != -60_600 {
		t.Fatalf("cash posting = %d, want -60600", fill.CashTransaction.Postings[0].Amount)
	}
	if fill.Execution.GrossCents != -60_600 {
		t.Fatalf("gross = %d, want -60600", fill.Execution.GrossCents)
	}
	if fill.Execution.CashTransactionID != fill.CashTransaction.ID {
		t.Fatalf("cash transaction id = %q, want %q", fill.Execution.CashTransactionID, fill.CashTransaction.ID)
	}
	if err := NewLedger().Post(fill.CashTransaction); err != nil {
		t.Fatalf("cash transaction is not balanced: %v", err)
	}
}

func TestFillOrderHonorsLimitBuyingPowerPositionAndFreshness(t *testing.T) {
	now := time.Date(2026, 8, 9, 14, 30, 0, 0, time.UTC)
	quote := ExecutableQuote{
		BidCents: 9_900,
		AskCents: 10_000,
		AsOf:     now.Add(-5 * time.Second),
		Source:   "licensed feed",
	}
	policy := FillPolicy{MaximumQuoteAge: 30 * time.Second}

	buyLimit, err := NewOrder(OrderRequest{
		ID:              "order-buy-limit",
		IdempotencyKey:  "buy-limit",
		AccountID:       "account-1",
		Symbol:          "AAPL",
		Side:            OrderSideBuy,
		Type:            OrderTypeLimit,
		Quantity:        1,
		LimitPriceCents: 9_999,
	}, now)
	if err != nil {
		t.Fatalf("create buy limit order: %v", err)
	}
	if _, err := FillOrder(buyLimit, quote, policy, 100_000, 0, now); !errors.Is(err, ErrLimitNotEligible) {
		t.Fatalf("buy limit error = %v, want ErrLimitNotEligible", err)
	}

	buyMarket, err := NewOrder(OrderRequest{
		ID:             "order-buy-market",
		IdempotencyKey: "buy-market",
		AccountID:      "account-1",
		Symbol:         "AAPL",
		Side:           OrderSideBuy,
		Type:           OrderTypeMarket,
		Quantity:       2,
	}, now)
	if err != nil {
		t.Fatalf("create buy market order: %v", err)
	}
	if _, err := FillOrder(buyMarket, quote, policy, 19_999, 0, now); !errors.Is(err, ErrInsufficientBuyingPower) {
		t.Fatalf("buying power error = %v, want ErrInsufficientBuyingPower", err)
	}

	sellMarket, err := NewOrder(OrderRequest{
		ID:             "order-sell-market",
		IdempotencyKey: "sell-market",
		AccountID:      "account-1",
		Symbol:         "AAPL",
		Side:           OrderSideSell,
		Type:           OrderTypeMarket,
		Quantity:       2,
	}, now)
	if err != nil {
		t.Fatalf("create sell market order: %v", err)
	}
	if _, err := FillOrder(sellMarket, quote, policy, 0, 1, now); !errors.Is(err, ErrInsufficientPosition) {
		t.Fatalf("position error = %v, want ErrInsufficientPosition", err)
	}
	if _, err := FillOrder(sellMarket, ExecutableQuote{
		BidCents: quote.BidCents,
		AskCents: quote.AskCents,
		AsOf:     now.Add(-31 * time.Second),
		Source:   quote.Source,
	}, policy, 0, 2, now); !errors.Is(err, ErrStaleQuote) {
		t.Fatalf("stale quote error = %v, want ErrStaleQuote", err)
	}
}

func TestNewOrderRejectsInvalidOrderDetails(t *testing.T) {
	_, err := NewOrder(OrderRequest{
		ID:              "order-1",
		IdempotencyKey:  "request-1",
		AccountID:       "account-1",
		Symbol:          "AAPL",
		Side:            OrderSideBuy,
		Type:            OrderTypeMarket,
		Quantity:        1,
		LimitPriceCents: 1,
	}, time.Now())
	if !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("market limit price error = %v, want ErrInvalidOrder", err)
	}
}
