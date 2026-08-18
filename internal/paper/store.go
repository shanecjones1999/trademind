package paper

import (
	"context"
	"errors"
	"time"
)

var ErrAccountNotFound = errors.New("paper account not found")

type AccountSnapshot struct {
	Account          PaperAccount `json:"account"`
	CashBalanceCents Money        `json:"cash_balance_cents"`
	Positions        []Position   `json:"positions"`
}

// LedgerActivityEntry is one account-level cash-ledger posting, in the
// account's own signed amount (deposits and sale proceeds are positive,
// purchases are negative).
type LedgerActivityEntry struct {
	TransactionID string    `json:"transaction_id"`
	OccurredAt    time.Time `json:"occurred_at"`
	Description   string    `json:"description"`
	AmountCents   Money     `json:"amount_cents"`
}

// AccountRepository persists paper accounts, cash ledger activity, and filled orders.
type AccountRepository interface {
	EnsureAccount(ctx context.Context, userID string) (PaperAccount, error)
	Snapshot(ctx context.Context, userID string) (AccountSnapshot, error)
	PostTransaction(ctx context.Context, transaction Transaction) error
	ApplyOrderFill(ctx context.Context, fill OrderFill) error
	ListActivity(ctx context.Context, userID string, limit int) ([]LedgerActivityEntry, error)
	ListOrders(ctx context.Context, userID string, limit, offset int) (OrderHistoryPage, error)
}
