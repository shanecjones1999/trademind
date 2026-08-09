package paper

import (
	"errors"
	"testing"
	"time"
)

func TestOpeningCashTransactionFundsPaperAccount(t *testing.T) {
	openedAt := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	account, err := NewPaperAccount("account-1", "google-user-1", openedAt)
	if err != nil {
		t.Fatalf("create paper account: %v", err)
	}
	transaction, err := OpeningCashTransaction(account, DefaultStartingCashCents)
	if err != nil {
		t.Fatalf("create opening transaction: %v", err)
	}

	ledger := NewLedger()
	if err := ledger.Post(transaction); err != nil {
		t.Fatalf("post opening transaction: %v", err)
	}
	balance, err := ledger.Balance(account.ID, USD)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != DefaultStartingCashCents {
		t.Fatalf("cash balance = %d, want %d", balance, DefaultStartingCashCents)
	}
	capitalBalance, err := ledger.Balance(paperCapitalAccountID, USD)
	if err != nil {
		t.Fatalf("get capital balance: %v", err)
	}
	if capitalBalance != -DefaultStartingCashCents {
		t.Fatalf("capital balance = %d, want %d", capitalBalance, -DefaultStartingCashCents)
	}
}

func TestLedgerRejectsUnbalancedAndDuplicateTransactions(t *testing.T) {
	ledger := NewLedger()
	transaction := Transaction{
		ID:          "unbalanced",
		OccurredAt:  time.Now(),
		Description: "Unbalanced transaction",
		Postings: []Posting{
			{AccountID: "account-1", Asset: USD, Amount: 100},
			{AccountID: "system", Asset: USD, Amount: -99},
		},
	}
	if err := ledger.Post(transaction); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("post unbalanced transaction error = %v, want ErrInvalidTransaction", err)
	}

	transaction.Postings[1].Amount = -100
	if err := ledger.Post(transaction); err != nil {
		t.Fatalf("post balanced transaction: %v", err)
	}
	if err := ledger.Post(transaction); !errors.Is(err, ErrDuplicateTransaction) {
		t.Fatalf("post duplicate transaction error = %v, want ErrDuplicateTransaction", err)
	}
}

func TestLedgerProtectsPostedTransactionHistory(t *testing.T) {
	ledger := NewLedger()
	transaction := Transaction{
		ID:          "fund-account",
		OccurredAt:  time.Now(),
		Description: "Fund account",
		Postings: []Posting{
			{AccountID: "account-1", Asset: USD, Amount: 100},
			{AccountID: "system", Asset: USD, Amount: -100},
		},
	}
	if err := ledger.Post(transaction); err != nil {
		t.Fatalf("post transaction: %v", err)
	}

	transaction.Postings[0].Amount = 500
	transactions := ledger.Transactions()
	transactions[0].Postings[0].Amount = 1

	balance, err := ledger.Balance("account-1", USD)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if balance != 100 {
		t.Fatalf("balance after caller mutations = %d, want 100", balance)
	}
}

func TestOpeningCashTransactionRejectsInvalidAmount(t *testing.T) {
	account, err := NewPaperAccount("account-1", "user-1", time.Now())
	if err != nil {
		t.Fatalf("create paper account: %v", err)
	}
	_, err = OpeningCashTransaction(account, 0)
	if !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("opening transaction error = %v, want ErrInvalidTransaction", err)
	}
}
