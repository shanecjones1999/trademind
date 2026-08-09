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

//go:embed migrations/0003_paper_positions.sql
var positionSchema string

//go:embed migrations/0004_paper_position_lots.sql
var positionLotSchema string

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
	if _, err := pool.Exec(ctx, positionSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-position schema: %w", err)
	}
	if _, err := pool.Exec(ctx, positionLotSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-position-lot schema: %w", err)
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
	positions, err := s.positionsByAccountID(ctx, snapshot.Account.ID)
	if err != nil {
		return AccountSnapshot{}, err
	}
	snapshot.Positions = positions
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

func (s *PostgresStore) ApplyOrderFill(ctx context.Context, fill OrderFill) error {
	if err := validateOrderFill(fill); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin order-fill transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := insertTransaction(ctx, tx, fill.CashTransaction); err != nil {
		return err
	}

	if fill.Order.Side == OrderSideBuy {
		if err := applyBuyOrderFill(ctx, tx, fill); err != nil {
			return err
		}
	} else {
		if err := applySellOrderFill(ctx, tx, fill); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit order fill: %w", err)
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

func (s *PostgresStore) positionsByAccountID(ctx context.Context, accountID string) ([]Position, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT symbol, quantity, cost_basis_cents, realized_pnl_cents
		 FROM paper_positions
		 WHERE account_id = $1
		 ORDER BY symbol ASC`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("list paper positions: %w", err)
	}
	defer rows.Close()

	positions := make([]Position, 0)
	for rows.Next() {
		var position Position
		var costBasis int64
		var realizedPnL int64
		if err := rows.Scan(
			&position.Symbol,
			&position.Quantity,
			&costBasis,
			&realizedPnL,
		); err != nil {
			return nil, fmt.Errorf("scan paper position: %w", err)
		}
		position.CostBasisCents = Money(costBasis)
		position.RealizedPnLCents = Money(realizedPnL)
		positions = append(positions, position)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate paper positions: %w", err)
	}
	return positions, nil
}

func applyBuyOrderFill(ctx context.Context, tx pgx.Tx, fill OrderFill) error {
	costBasis, err := multipliedMoney(fill.Execution.PriceCents, fill.Execution.Quantity)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO paper_positions (account_id, symbol, quantity, cost_basis_cents, realized_pnl_cents)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (account_id, symbol) DO UPDATE
		 SET quantity = paper_positions.quantity + EXCLUDED.quantity,
		     cost_basis_cents = paper_positions.cost_basis_cents + EXCLUDED.cost_basis_cents`,
		fill.Order.AccountID,
		fill.Order.Symbol,
		fill.Execution.Quantity,
		int64(costBasis),
		int64(0),
	); err != nil {
		return fmt.Errorf("upsert paper position: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO paper_position_lots (account_id, symbol, quantity, cost_per_share_cents, acquired_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		fill.Order.AccountID,
		fill.Order.Symbol,
		fill.Execution.Quantity,
		int64(fill.Execution.PriceCents),
		fill.Execution.OccurredAt.UTC(),
	); err != nil {
		return fmt.Errorf("insert paper position lot: %w", err)
	}
	return nil
}

func applySellOrderFill(ctx context.Context, tx pgx.Tx, fill OrderFill) error {
	lotRows, err := tx.Query(
		ctx,
		`SELECT id, quantity, cost_per_share_cents
		 FROM paper_position_lots
		 WHERE account_id = $1 AND symbol = $2
		 ORDER BY acquired_at ASC, id ASC
		 FOR UPDATE`,
		fill.Order.AccountID,
		fill.Order.Symbol,
	)
	if err != nil {
		return fmt.Errorf("load paper position lots: %w", err)
	}
	defer lotRows.Close()

	type storedLot struct {
		id           int64
		quantity     int64
		costPerShare Money
	}

	lots := make([]storedLot, 0)
	for lotRows.Next() {
		var lot storedLot
		var costPerShare int64
		if err := lotRows.Scan(&lot.id, &lot.quantity, &costPerShare); err != nil {
			return fmt.Errorf("scan paper position lot: %w", err)
		}
		lot.costPerShare = Money(costPerShare)
		lots = append(lots, lot)
	}
	if err := lotRows.Err(); err != nil {
		return fmt.Errorf("iterate paper position lots: %w", err)
	}

	remainingToSell := fill.Execution.Quantity
	consumedCost := Money(0)
	for index := range lots {
		consumed := min(lots[index].quantity, remainingToSell)
		if consumed == 0 {
			continue
		}
		consumedLotCost, err := multipliedMoney(lots[index].costPerShare, consumed)
		if err != nil {
			return err
		}
		consumedCost += consumedLotCost
		lots[index].quantity -= consumed
		remainingToSell -= consumed
	}
	if remainingToSell != 0 {
		return ErrInsufficientPosition
	}
	proceeds, err := multipliedMoney(fill.Execution.PriceCents, fill.Execution.Quantity)
	if err != nil {
		return err
	}
	realizedPnL := proceeds - consumedCost
	remainingQuantity := int64(0)
	remainingCostBasis := Money(0)
	for _, lot := range lots {
		if lot.quantity == 0 {
			continue
		}
		remainingQuantity += lot.quantity
		lotCost, err := multipliedMoney(lot.costPerShare, lot.quantity)
		if err != nil {
			return err
		}
		remainingCostBasis += lotCost
	}

	var currentRealizedPnL int64
	err = tx.QueryRow(
		ctx,
		`SELECT realized_pnl_cents
		 FROM paper_positions
		 WHERE account_id = $1 AND symbol = $2
		 FOR UPDATE`,
		fill.Order.AccountID,
		fill.Order.Symbol,
	).Scan(&currentRealizedPnL)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInsufficientPosition
	}
	if err != nil {
		return fmt.Errorf("load paper position summary: %w", err)
	}
	nextRealizedPnL := Money(currentRealizedPnL) + realizedPnL

	if remainingQuantity == 0 {
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM paper_positions WHERE account_id = $1 AND symbol = $2`,
			fill.Order.AccountID,
			fill.Order.Symbol,
		); err != nil {
			return fmt.Errorf("delete closed paper position: %w", err)
		}
	} else {
		if _, err := tx.Exec(
			ctx,
			`UPDATE paper_positions
			 SET quantity = $3,
			     cost_basis_cents = $4,
			     realized_pnl_cents = $5
			 WHERE account_id = $1 AND symbol = $2`,
			fill.Order.AccountID,
			fill.Order.Symbol,
			remainingQuantity,
			int64(remainingCostBasis),
			int64(nextRealizedPnL),
		); err != nil {
			return fmt.Errorf("update paper position: %w", err)
		}
	}

	for _, lot := range lots {
		if lot.quantity > 0 {
			if _, err := tx.Exec(
				ctx,
				`UPDATE paper_position_lots
				 SET quantity = $2
				 WHERE id = $1`,
				lot.id,
				lot.quantity,
			); err != nil {
				return fmt.Errorf("update paper position lot: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM paper_position_lots WHERE id = $1`,
			lot.id,
		); err != nil {
			return fmt.Errorf("delete paper position lot: %w", err)
		}
	}

	return nil
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
