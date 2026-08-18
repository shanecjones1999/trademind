package paper

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shanecjones1999/trademind/internal/market"
)

const paperMarketAccountID = "system:paper-market"

var (
	ErrInvalidOrder            = errors.New("invalid paper order")
	ErrStaleQuote              = errors.New("quote is stale")
	ErrLimitNotEligible        = errors.New("limit order is not eligible to fill")
	ErrInsufficientBuyingPower = errors.New("insufficient buying power")
	ErrInsufficientPosition    = errors.New("insufficient position quantity")
)

type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
)

type OrderStatus string

const (
	OrderStatusOpen   OrderStatus = "open"
	OrderStatusFilled OrderStatus = "filled"
)

type OrderRequest struct {
	ID              string
	IdempotencyKey  string
	AccountID       string
	Symbol          string
	Side            OrderSide
	Type            OrderType
	Quantity        int64
	LimitPriceCents Money
}

type Order struct {
	ID              string      `json:"id"`
	IdempotencyKey  string      `json:"idempotency_key"`
	AccountID       string      `json:"account_id"`
	Symbol          string      `json:"symbol"`
	Side            OrderSide   `json:"side"`
	Type            OrderType   `json:"type"`
	Quantity        int64       `json:"quantity"`
	LimitPriceCents Money       `json:"limit_price_cents,omitempty"`
	Status          OrderStatus `json:"status"`
	SubmittedAt     time.Time   `json:"submitted_at"`
}

type ExecutableQuote struct {
	BidCents Money
	AskCents Money
	AsOf     time.Time
	Source   string
}

type FillPolicy struct {
	MaximumQuoteAge time.Duration
	SlippageBps     int64
}

type Execution struct {
	ID                string    `json:"id"`
	OrderID           string    `json:"order_id"`
	Symbol            string    `json:"symbol"`
	Quantity          int64     `json:"quantity"`
	PriceCents        Money     `json:"price_cents"`
	OccurredAt        time.Time `json:"occurred_at"`
	QuoteAsOf         time.Time `json:"quote_as_of"`
	QuoteSource       string    `json:"quote_source"`
	GrossCents        Money     `json:"gross_cents"`
	RealizedPnLCents  *Money    `json:"realized_pnl_cents"`
	CashTransactionID string    `json:"cash_transaction_id,omitempty"`
}

// OrderHistoryEntry is a filled order plus its execution, newest first in lists.
type OrderHistoryEntry struct {
	Order
	Execution Execution `json:"execution"`
}

type OrderHistoryPage struct {
	Orders []OrderHistoryEntry `json:"orders"`
	Total  int                 `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type OrderFill struct {
	Order           Order       `json:"order"`
	Execution       Execution   `json:"execution"`
	CashTransaction Transaction `json:"cash_transaction"`
}

func NewOrder(request OrderRequest, submittedAt time.Time) (Order, error) {
	if strings.TrimSpace(request.ID) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" ||
		strings.TrimSpace(request.AccountID) == "" ||
		submittedAt.IsZero() {
		return Order{}, fmt.Errorf("%w: ID, idempotency key, account ID, and submission time are required", ErrInvalidOrder)
	}
	if len(request.IdempotencyKey) > 255 {
		return Order{}, fmt.Errorf("%w: idempotency key exceeds 255 characters", ErrInvalidOrder)
	}
	symbol, err := market.NormalizeSymbol(request.Symbol)
	if err != nil {
		return Order{}, fmt.Errorf("%w: symbol is invalid", ErrInvalidOrder)
	}
	if request.Quantity <= 0 {
		return Order{}, fmt.Errorf("%w: quantity must be positive", ErrInvalidOrder)
	}
	if request.Side != OrderSideBuy && request.Side != OrderSideSell {
		return Order{}, fmt.Errorf("%w: side must be buy or sell", ErrInvalidOrder)
	}
	switch request.Type {
	case OrderTypeMarket:
		if request.LimitPriceCents != 0 {
			return Order{}, fmt.Errorf("%w: market orders cannot set a limit price", ErrInvalidOrder)
		}
	case OrderTypeLimit:
		if request.LimitPriceCents <= 0 {
			return Order{}, fmt.Errorf("%w: limit orders require a positive limit price", ErrInvalidOrder)
		}
	default:
		return Order{}, fmt.Errorf("%w: order type is invalid", ErrInvalidOrder)
	}

	return Order{
		ID:              request.ID,
		IdempotencyKey:  request.IdempotencyKey,
		AccountID:       request.AccountID,
		Symbol:          symbol,
		Side:            request.Side,
		Type:            request.Type,
		Quantity:        request.Quantity,
		LimitPriceCents: request.LimitPriceCents,
		Status:          OrderStatusOpen,
		SubmittedAt:     submittedAt.UTC(),
	}, nil
}

func FillOrder(order Order, quote ExecutableQuote, policy FillPolicy, cashBalance Money, availableQuantity int64, now time.Time) (OrderFill, error) {
	if order.Status != OrderStatusOpen {
		return OrderFill{}, fmt.Errorf("%w: order must be open", ErrInvalidOrder)
	}
	if cashBalance < 0 || availableQuantity < 0 || now.IsZero() {
		return OrderFill{}, fmt.Errorf("%w: balances and fill time are invalid", ErrInvalidOrder)
	}
	if err := validateQuote(quote, policy, now); err != nil {
		return OrderFill{}, err
	}

	price, err := executablePrice(order.Side, quote, policy.SlippageBps)
	if err != nil {
		return OrderFill{}, err
	}
	if !isLimitEligible(order, price) {
		return OrderFill{}, ErrLimitNotEligible
	}
	gross, err := multipliedMoney(price, order.Quantity)
	if err != nil {
		return OrderFill{}, err
	}
	if order.Side == OrderSideBuy && gross > cashBalance {
		return OrderFill{}, ErrInsufficientBuyingPower
	}
	if order.Side == OrderSideSell && order.Quantity > availableQuantity {
		return OrderFill{}, ErrInsufficientPosition
	}

	filledOrder := order
	filledOrder.Status = OrderStatusFilled
	execution := Execution{
		ID:          "execution:" + order.ID,
		OrderID:     order.ID,
		Symbol:      order.Symbol,
		Quantity:    order.Quantity,
		PriceCents:  price,
		OccurredAt:  now.UTC(),
		QuoteAsOf:   quote.AsOf.UTC(),
		QuoteSource: quote.Source,
	}
	transaction, err := cashTransactionForExecution(order, gross, execution.OccurredAt)
	if err != nil {
		return OrderFill{}, err
	}
	execution.GrossCents = transaction.Postings[0].Amount
	execution.CashTransactionID = transaction.ID
	return OrderFill{
		Order:           filledOrder,
		Execution:       execution,
		CashTransaction: transaction,
	}, nil
}

func validateQuote(quote ExecutableQuote, policy FillPolicy, now time.Time) error {
	if quote.BidCents <= 0 || quote.AskCents <= 0 || quote.BidCents > quote.AskCents ||
		quote.AsOf.IsZero() || strings.TrimSpace(quote.Source) == "" {
		return fmt.Errorf("%w: quote is invalid", ErrInvalidOrder)
	}
	if policy.MaximumQuoteAge <= 0 || policy.SlippageBps < 0 || policy.SlippageBps > 1_000 {
		return fmt.Errorf("%w: fill policy is invalid", ErrInvalidOrder)
	}
	if now.Before(quote.AsOf) || now.Sub(quote.AsOf) > policy.MaximumQuoteAge {
		return ErrStaleQuote
	}
	return nil
}

func executablePrice(side OrderSide, quote ExecutableQuote, slippageBps int64) (Money, error) {
	basePrice := quote.AskCents
	if side == OrderSideSell {
		basePrice = quote.BidCents
	}
	if slippageBps == 0 {
		return basePrice, nil
	}
	slippage, err := multipliedMoney(basePrice, slippageBps)
	if err != nil {
		return 0, err
	}
	slippage = (slippage + 9_999) / 10_000
	if side == OrderSideBuy {
		if basePrice > Money(math.MaxInt64)-slippage {
			return 0, fmt.Errorf("%w: execution price overflows", ErrInvalidOrder)
		}
		return basePrice + slippage, nil
	}
	if basePrice <= slippage {
		return 0, fmt.Errorf("%w: execution price must be positive", ErrInvalidOrder)
	}
	return basePrice - slippage, nil
}

func isLimitEligible(order Order, price Money) bool {
	if order.Type == OrderTypeMarket {
		return true
	}
	if order.Side == OrderSideBuy {
		return price <= order.LimitPriceCents
	}
	return price >= order.LimitPriceCents
}

func multipliedMoney(amount Money, multiplier int64) (Money, error) {
	if amount <= 0 || multiplier <= 0 || amount > Money(math.MaxInt64/multiplier) {
		return 0, fmt.Errorf("%w: amount exceeds supported range", ErrInvalidOrder)
	}
	return amount * Money(multiplier), nil
}

func cashTransactionForExecution(order Order, gross Money, occurredAt time.Time) (Transaction, error) {
	accountAmount := -gross
	description := "Paper purchase of " + order.Symbol
	if order.Side == OrderSideSell {
		accountAmount = gross
		description = "Paper sale of " + order.Symbol
	}
	transaction := Transaction{
		ID:          "order-fill:" + order.ID,
		OccurredAt:  occurredAt,
		Description: description,
		Postings: []Posting{
			{AccountID: order.AccountID, Asset: USD, Amount: accountAmount},
			{AccountID: paperMarketAccountID, Asset: USD, Amount: -accountAmount},
		},
	}
	if err := validateTransaction(transaction); err != nil {
		return Transaction{}, err
	}
	return transaction, nil
}
