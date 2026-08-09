package paper

import (
	"context"
	"errors"
)

var ErrAccountNotFound = errors.New("paper account not found")

type AccountSnapshot struct {
	Account          PaperAccount `json:"account"`
	CashBalanceCents Money        `json:"cash_balance_cents"`
}

// AccountRepository persists paper accounts and their cash ledger activity.
type AccountRepository interface {
	EnsureAccount(ctx context.Context, userID string) (PaperAccount, error)
	Snapshot(ctx context.Context, userID string) (AccountSnapshot, error)
	PostTransaction(ctx context.Context, transaction Transaction) error
}
