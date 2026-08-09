package paper

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/0001_paper_accounts.sql
var initialSchema string

//go:embed migrations/0002_watchlists.sql
var watchlistSchema string

type PostgresStore struct {
	pool  *pgxpool.Pool
	now   func() time.Time
	newID func(prefix string) (string, error)
}

func OpenPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	if _, err := pool.Exec(ctx, initialSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-account schema: %w", err)
	}
	if _, err := pool.Exec(ctx, watchlistSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply watchlist schema: %w", err)
	}

	return &PostgresStore{
		pool:  pool,
		now:   time.Now,
		newID: newIdentifier,
	}, nil
}

func (s *PostgresStore) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresStore) EnsureAccount(ctx context.Context, userID string) (PaperAccount, error) {
	if strings.TrimSpace(userID) == "" {
		return PaperAccount{}, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PaperAccount{}, fmt.Errorf("begin account transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	account, err := queryAccount(ctx, tx, userID)
	switch {
	case err == nil:
		if err := tx.Commit(ctx); err != nil {
			return PaperAccount{}, fmt.Errorf("commit account lookup: %w", err)
		}
		return account, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return PaperAccount{}, fmt.Errorf("find paper account: %w", err)
	}

	accountID, err := s.newID("paper-account")
	if err != nil {
		return PaperAccount{}, fmt.Errorf("generate paper account ID: %w", err)
	}
	account, err = NewPaperAccount(accountID, userID, s.now())
	if err != nil {
		return PaperAccount{}, err
	}
	openingTransaction, err := OpeningCashTransaction(account, DefaultStartingCashCents)
	if err != nil {
		return PaperAccount{}, err
	}

	inserted, err := tx.Exec(
		ctx,
		`INSERT INTO paper_accounts (id, user_id, opened_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO NOTHING`,
		account.ID,
		account.UserID,
		account.OpenedAt,
	)
	if err != nil {
		return PaperAccount{}, fmt.Errorf("insert paper account: %w", err)
	}
	if inserted.RowsAffected() == 0 {
		account, err = queryAccount(ctx, tx, userID)
		if err != nil {
			return PaperAccount{}, fmt.Errorf("load concurrently created paper account: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return PaperAccount{}, fmt.Errorf("commit account lookup: %w", err)
		}
		return account, nil
	}

	if err := insertTransaction(ctx, tx, openingTransaction); err != nil {
		return PaperAccount{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PaperAccount{}, fmt.Errorf("commit paper account: %w", err)
	}
	return account, nil
}

func (s *PostgresStore) Snapshot(ctx context.Context, userID string) (AccountSnapshot, error) {
	if strings.TrimSpace(userID) == "" {
		return AccountSnapshot{}, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}

	var snapshot AccountSnapshot
	var balance int64
	err := s.pool.QueryRow(
		ctx,
		`SELECT accounts.id, accounts.user_id, accounts.opened_at,
		        COALESCE(SUM(entries.amount_cents), 0)
		 FROM paper_accounts AS accounts
		 LEFT JOIN cash_ledger_entries AS entries
		   ON entries.account_id = accounts.id AND entries.asset = $2
		 WHERE accounts.user_id = $1
		 GROUP BY accounts.id, accounts.user_id, accounts.opened_at`,
		userID,
		USD,
	).Scan(
		&snapshot.Account.ID,
		&snapshot.Account.UserID,
		&snapshot.Account.OpenedAt,
		&balance,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountSnapshot{}, ErrAccountNotFound
	}
	if err != nil {
		return AccountSnapshot{}, fmt.Errorf("load paper account snapshot: %w", err)
	}
	snapshot.Account.OpenedAt = snapshot.Account.OpenedAt.UTC()
	snapshot.CashBalanceCents = Money(balance)
	return snapshot, nil
}

func (s *PostgresStore) PostTransaction(ctx context.Context, transaction Transaction) error {
	if err := validateTransaction(transaction); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ledger transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertTransaction(ctx, tx, transaction); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ledger transaction: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListWatchlists(ctx context.Context, userID string) ([]Watchlist, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT watchlists.id, watchlists.user_id, watchlists.name, watchlists.created_at,
			        symbols.symbol, symbols.added_at
			 FROM watchlists
			 LEFT JOIN watchlist_symbols AS symbols ON symbols.watchlist_id = watchlists.id
			 WHERE watchlists.user_id = $1
			 ORDER BY watchlists.created_at DESC, symbols.added_at ASC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list watchlists: %w", err)
	}
	defer rows.Close()

	watchlists := make([]Watchlist, 0)
	byID := make(map[string]int)
	for rows.Next() {
		watchlist := Watchlist{Symbols: []WatchlistSymbol{}}
		var symbol *string
		var addedAt *time.Time
		if err := rows.Scan(
			&watchlist.ID,
			&watchlist.UserID,
			&watchlist.Name,
			&watchlist.CreatedAt,
			&symbol,
			&addedAt,
		); err != nil {
			return nil, fmt.Errorf("scan watchlist: %w", err)
		}
		watchlist.CreatedAt = watchlist.CreatedAt.UTC()

		index, exists := byID[watchlist.ID]
		if !exists {
			index = len(watchlists)
			byID[watchlist.ID] = index
			watchlists = append(watchlists, watchlist)
		}
		if symbol != nil && addedAt != nil {
			watchlists[index].Symbols = append(watchlists[index].Symbols, WatchlistSymbol{
				Symbol:  *symbol,
				AddedAt: addedAt.UTC(),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchlists: %w", err)
	}
	return watchlists, nil
}

func (s *PostgresStore) CreateWatchlist(ctx context.Context, userID, name string) (Watchlist, error) {
	if strings.TrimSpace(userID) == "" {
		return Watchlist{}, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}
	name, err := validateWatchlistName(name)
	if err != nil {
		return Watchlist{}, err
	}
	id, err := s.newID("watchlist")
	if err != nil {
		return Watchlist{}, fmt.Errorf("generate watchlist ID: %w", err)
	}
	watchlist := Watchlist{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Symbols:   []WatchlistSymbol{},
		CreatedAt: s.now().UTC(),
	}
	if _, err := s.pool.Exec(
		ctx,
		`INSERT INTO watchlists (id, user_id, name, created_at) VALUES ($1, $2, $3, $4)`,
		watchlist.ID,
		watchlist.UserID,
		watchlist.Name,
		watchlist.CreatedAt,
	); err != nil {
		return Watchlist{}, fmt.Errorf("create watchlist: %w", err)
	}
	return watchlist, nil
}

func (s *PostgresStore) AddWatchlistSymbol(ctx context.Context, userID, watchlistID, symbol string) (WatchlistSymbol, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(watchlistID) == "" || strings.TrimSpace(symbol) == "" {
		return WatchlistSymbol{}, fmt.Errorf("%w: user ID, watchlist ID, and symbol are required", ErrInvalidTransaction)
	}

	watchlistSymbol := WatchlistSymbol{Symbol: symbol, AddedAt: s.now().UTC()}
	command, err := s.pool.Exec(
		ctx,
		`INSERT INTO watchlist_symbols (watchlist_id, symbol, added_at)
			 SELECT id, $3, $4 FROM watchlists WHERE id = $1 AND user_id = $2`,
		watchlistID,
		userID,
		watchlistSymbol.Symbol,
		watchlistSymbol.AddedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return WatchlistSymbol{}, fmt.Errorf("%w: %s", ErrDuplicateWatchlistSymbol, symbol)
		}
		return WatchlistSymbol{}, fmt.Errorf("add watchlist symbol: %w", err)
	}
	if command.RowsAffected() == 0 {
		return WatchlistSymbol{}, ErrWatchlistNotFound
	}
	return watchlistSymbol, nil
}

func (s *PostgresStore) RemoveWatchlistSymbol(ctx context.Context, userID, watchlistID, symbol string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(watchlistID) == "" || strings.TrimSpace(symbol) == "" {
		return fmt.Errorf("%w: user ID, watchlist ID, and symbol are required", ErrInvalidTransaction)
	}

	command, err := s.pool.Exec(
		ctx,
		`DELETE FROM watchlist_symbols
			 WHERE watchlist_id = $1
			   AND symbol = $3
			   AND EXISTS (
			       SELECT 1 FROM watchlists WHERE id = $1 AND user_id = $2
			   )`,
		watchlistID,
		userID,
		symbol,
	)
	if err != nil {
		return fmt.Errorf("remove watchlist symbol: %w", err)
	}
	if command.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM watchlists WHERE id = $1 AND user_id = $2)`,
			watchlistID,
			userID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check watchlist ownership: %w", err)
		}
		if !exists {
			return ErrWatchlistNotFound
		}
	}
	return nil
}

func queryAccount(ctx context.Context, tx pgx.Tx, userID string) (PaperAccount, error) {
	var account PaperAccount
	err := tx.QueryRow(
		ctx,
		`SELECT id, user_id, opened_at
		 FROM paper_accounts
		 WHERE user_id = $1`,
		userID,
	).Scan(&account.ID, &account.UserID, &account.OpenedAt)
	account.OpenedAt = account.OpenedAt.UTC()
	return account, err
}

func insertTransaction(ctx context.Context, tx pgx.Tx, transaction Transaction) error {
	if err := validateTransaction(transaction); err != nil {
		return err
	}
	for index, posting := range transaction.Postings {
		_, err := tx.Exec(
			ctx,
			`INSERT INTO cash_ledger_entries
			 (transaction_id, entry_index, account_id, asset, amount_cents, occurred_at, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			transaction.ID,
			index,
			posting.AccountID,
			posting.Asset,
			int64(posting.Amount),
			transaction.OccurredAt,
			transaction.Description,
		)
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: %s", ErrDuplicateTransaction, transaction.ID)
			}
			return fmt.Errorf("insert cash ledger entry: %w", err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var databaseError *pgconn.PgError
	return errors.As(err, &databaseError) && databaseError.Code == "23505"
}

func newIdentifier(prefix string) (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes), nil
}
