// Package paper contains the domain model for simulated trading accounts.
package paper

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	USD                      = "USD"
	DefaultStartingCashCents = Money(10_000_000)
	paperCapitalAccountID    = "system:paper-capital"
)

var (
	ErrInvalidTransaction   = errors.New("invalid ledger transaction")
	ErrDuplicateTransaction = errors.New("duplicate ledger transaction")
)

// Money represents an exact amount in the smallest unit of its asset.
// For USD, one unit is one cent.
type Money int64

type PaperAccount struct {
	ID       string    `json:"id"`
	UserID   string    `json:"user_id"`
	OpenedAt time.Time `json:"opened_at"`
}

type Posting struct {
	AccountID string
	Asset     string
	Amount    Money
}

type Transaction struct {
	ID          string
	OccurredAt  time.Time
	Description string
	Postings    []Posting
}

// Ledger stores balanced transactions. Posted transactions are copied so
// callers cannot mutate historical account activity after it is recorded.
type Ledger struct {
	transactions map[string]Transaction
	order        []string
}

func NewLedger() *Ledger {
	return &Ledger{
		transactions: make(map[string]Transaction),
	}
}

func NewPaperAccount(id, userID string, openedAt time.Time) (PaperAccount, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" || openedAt.IsZero() {
		return PaperAccount{}, fmt.Errorf("%w: account ID, user ID, and opening time are required", ErrInvalidTransaction)
	}

	return PaperAccount{
		ID:       id,
		UserID:   userID,
		OpenedAt: openedAt.UTC(),
	}, nil
}

func OpeningCashTransaction(account PaperAccount, amount Money) (Transaction, error) {
	if account.ID == "" || account.UserID == "" || account.OpenedAt.IsZero() {
		return Transaction{}, fmt.Errorf("%w: account is incomplete", ErrInvalidTransaction)
	}
	if amount <= 0 {
		return Transaction{}, fmt.Errorf("%w: opening cash must be positive", ErrInvalidTransaction)
	}

	return Transaction{
		ID:          "opening-cash:" + account.ID,
		OccurredAt:  account.OpenedAt,
		Description: "Initial paper cash",
		Postings: []Posting{
			{AccountID: account.ID, Asset: USD, Amount: amount},
			{AccountID: paperCapitalAccountID, Asset: USD, Amount: -amount},
		},
	}, nil
}

func (l *Ledger) Post(transaction Transaction) error {
	if l == nil {
		return fmt.Errorf("%w: ledger is required", ErrInvalidTransaction)
	}
	if err := validateTransaction(transaction); err != nil {
		return err
	}
	if _, exists := l.transactions[transaction.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTransaction, transaction.ID)
	}

	l.transactions[transaction.ID] = cloneTransaction(transaction)
	l.order = append(l.order, transaction.ID)
	return nil
}

func (l *Ledger) Balance(accountID, asset string) (Money, error) {
	if l == nil || strings.TrimSpace(accountID) == "" || asset != USD {
		return 0, fmt.Errorf("%w: account ID and supported asset are required", ErrInvalidTransaction)
	}

	var balance Money
	for _, transactionID := range l.order {
		for _, posting := range l.transactions[transactionID].Postings {
			if posting.AccountID == accountID && posting.Asset == asset {
				balance += posting.Amount
			}
		}
	}
	return balance, nil
}

func (l *Ledger) Transactions() []Transaction {
	if l == nil {
		return nil
	}

	transactions := make([]Transaction, 0, len(l.order))
	for _, transactionID := range l.order {
		transactions = append(transactions, cloneTransaction(l.transactions[transactionID]))
	}
	return transactions
}

func validateTransaction(transaction Transaction) error {
	if strings.TrimSpace(transaction.ID) == "" || strings.TrimSpace(transaction.Description) == "" || transaction.OccurredAt.IsZero() {
		return fmt.Errorf("%w: ID, description, and occurrence time are required", ErrInvalidTransaction)
	}
	if len(transaction.Postings) < 2 {
		return fmt.Errorf("%w: at least two postings are required", ErrInvalidTransaction)
	}

	totals := make(map[string]Money)
	for _, posting := range transaction.Postings {
		if strings.TrimSpace(posting.AccountID) == "" || posting.Asset != USD || posting.Amount == 0 {
			return fmt.Errorf("%w: postings need an account, USD asset, and non-zero amount", ErrInvalidTransaction)
		}
		totals[posting.Asset] += posting.Amount
	}
	for asset, total := range totals {
		if total != 0 {
			return fmt.Errorf("%w: %s postings do not balance", ErrInvalidTransaction, asset)
		}
	}
	return nil
}

func cloneTransaction(transaction Transaction) Transaction {
	copy := transaction
	copy.Postings = append([]Posting(nil), transaction.Postings...)
	return copy
}
