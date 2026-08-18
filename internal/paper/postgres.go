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

//go:embed migrations/0003_paper_positions.sql
var positionSchema string

//go:embed migrations/0004_paper_position_lots.sql
var positionLotSchema string

//go:embed migrations/0005_orders.sql
var orderSchema string

//go:embed migrations/0006_drop_watchlists.sql
var dropWatchlistSchema string

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
	if _, err := pool.Exec(ctx, positionSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-position schema: %w", err)
	}
	if _, err := pool.Exec(ctx, positionLotSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-position-lot schema: %w", err)
	}
	if _, err := pool.Exec(ctx, orderSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply paper-order schema: %w", err)
	}
	if _, err := pool.Exec(ctx, dropWatchlistSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("drop watchlist schema: %w", err)
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

	var realizedPnL *Money
	if fill.Order.Side == OrderSideBuy {
		if err := applyBuyOrderFill(ctx, tx, fill); err != nil {
			return err
		}
	} else {
		realized, err := applySellOrderFill(ctx, tx, fill)
		if err != nil {
			return err
		}
		realizedPnL = &realized
	}
	if err := insertFilledOrder(ctx, tx, fill, realizedPnL); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit order fill: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListActivity(ctx context.Context, userID string, limit int) ([]LedgerActivityEntry, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT entries.transaction_id, entries.occurred_at, entries.description, entries.amount_cents
		 FROM cash_ledger_entries AS entries
		 JOIN paper_accounts AS accounts ON accounts.id = entries.account_id
		 WHERE accounts.user_id = $1 AND entries.asset = $2
		 ORDER BY entries.occurred_at DESC, entries.transaction_id DESC
		 LIMIT $3`,
		userID,
		USD,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list account activity: %w", err)
	}
	defer rows.Close()

	activity := make([]LedgerActivityEntry, 0)
	for rows.Next() {
		var entry LedgerActivityEntry
		var amount int64
		if err := rows.Scan(&entry.TransactionID, &entry.OccurredAt, &entry.Description, &amount); err != nil {
			return nil, fmt.Errorf("scan account activity: %w", err)
		}
		entry.OccurredAt = entry.OccurredAt.UTC()
		entry.AmountCents = Money(amount)
		activity = append(activity, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate account activity: %w", err)
	}
	return activity, nil
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

func applySellOrderFill(ctx context.Context, tx pgx.Tx, fill OrderFill) (Money, error) {
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
		return 0, fmt.Errorf("load paper position lots: %w", err)
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
			return 0, fmt.Errorf("scan paper position lot: %w", err)
		}
		lot.costPerShare = Money(costPerShare)
		lots = append(lots, lot)
	}
	if err := lotRows.Err(); err != nil {
		return 0, fmt.Errorf("iterate paper position lots: %w", err)
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
			return 0, err
		}
		consumedCost += consumedLotCost
		lots[index].quantity -= consumed
		remainingToSell -= consumed
	}
	if remainingToSell != 0 {
		return 0, ErrInsufficientPosition
	}
	proceeds, err := multipliedMoney(fill.Execution.PriceCents, fill.Execution.Quantity)
	if err != nil {
		return 0, err
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
			return 0, err
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
		return 0, ErrInsufficientPosition
	}
	if err != nil {
		return 0, fmt.Errorf("load paper position summary: %w", err)
	}
	nextRealizedPnL := Money(currentRealizedPnL) + realizedPnL

	if remainingQuantity == 0 {
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM paper_positions WHERE account_id = $1 AND symbol = $2`,
			fill.Order.AccountID,
			fill.Order.Symbol,
		); err != nil {
			return 0, fmt.Errorf("delete closed paper position: %w", err)
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
			return 0, fmt.Errorf("update paper position: %w", err)
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
				return 0, fmt.Errorf("update paper position lot: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(
			ctx,
			`DELETE FROM paper_position_lots WHERE id = $1`,
			lot.id,
		); err != nil {
			return 0, fmt.Errorf("delete paper position lot: %w", err)
		}
	}

	return realizedPnL, nil
}

func insertFilledOrder(ctx context.Context, tx pgx.Tx, fill OrderFill, realizedPnL *Money) error {
	var limitPrice any
	if fill.Order.LimitPriceCents > 0 {
		limitPrice = int64(fill.Order.LimitPriceCents)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO paper_orders
		 (id, account_id, idempotency_key, symbol, side, order_type, quantity, limit_price_cents, status, submitted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		fill.Order.ID,
		fill.Order.AccountID,
		fill.Order.IdempotencyKey,
		fill.Order.Symbol,
		string(fill.Order.Side),
		string(fill.Order.Type),
		fill.Order.Quantity,
		limitPrice,
		string(fill.Order.Status),
		fill.Order.SubmittedAt.UTC(),
	); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", ErrDuplicateTransaction, fill.Order.ID)
		}
		return fmt.Errorf("insert paper order: %w", err)
	}

	grossCents := fill.Execution.GrossCents
	if grossCents == 0 && len(fill.CashTransaction.Postings) > 0 {
		grossCents = fill.CashTransaction.Postings[0].Amount
	}
	cashTransactionID := fill.Execution.CashTransactionID
	if cashTransactionID == "" {
		cashTransactionID = fill.CashTransaction.ID
	}
	var realized any
	if realizedPnL != nil {
		realized = int64(*realizedPnL)
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO paper_executions
		 (id, order_id, symbol, quantity, price_cents, occurred_at, quote_as_of, quote_source,
		  gross_cents, realized_pnl_cents, cash_transaction_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		fill.Execution.ID,
		fill.Order.ID,
		fill.Execution.Symbol,
		fill.Execution.Quantity,
		int64(fill.Execution.PriceCents),
		fill.Execution.OccurredAt.UTC(),
		fill.Execution.QuoteAsOf.UTC(),
		fill.Execution.QuoteSource,
		int64(grossCents),
		realized,
		cashTransactionID,
	); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", ErrDuplicateTransaction, fill.Execution.ID)
		}
		return fmt.Errorf("insert paper execution: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListOrders(ctx context.Context, userID string, limit, offset int) (OrderHistoryPage, error) {
	if strings.TrimSpace(userID) == "" {
		return OrderHistoryPage{}, fmt.Errorf("%w: user ID is required", ErrInvalidTransaction)
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := s.pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM paper_orders AS orders
		 JOIN paper_executions AS executions ON executions.order_id = orders.id
		 JOIN paper_accounts AS accounts ON accounts.id = orders.account_id
		 WHERE accounts.user_id = $1`,
		userID,
	).Scan(&total); err != nil {
		return OrderHistoryPage{}, fmt.Errorf("count paper orders: %w", err)
	}

	rows, err := s.pool.Query(
		ctx,
		`SELECT orders.id, orders.account_id, orders.idempotency_key, orders.symbol,
		        orders.side, orders.order_type, orders.quantity, orders.limit_price_cents,
		        orders.status, orders.submitted_at,
		        executions.id, executions.order_id, executions.symbol, executions.quantity,
		        executions.price_cents, executions.occurred_at, executions.quote_as_of,
		        executions.quote_source, executions.gross_cents, executions.realized_pnl_cents,
		        executions.cash_transaction_id
		 FROM paper_orders AS orders
		 JOIN paper_executions AS executions ON executions.order_id = orders.id
		 JOIN paper_accounts AS accounts ON accounts.id = orders.account_id
		 WHERE accounts.user_id = $1
		 ORDER BY executions.occurred_at DESC, executions.id DESC
		 LIMIT $2 OFFSET $3`,
		userID,
		limit,
		offset,
	)
	if err != nil {
		return OrderHistoryPage{}, fmt.Errorf("list paper orders: %w", err)
	}
	defer rows.Close()

	orders := make([]OrderHistoryEntry, 0)
	for rows.Next() {
		var entry OrderHistoryEntry
		var limitPrice *int64
		var priceCents int64
		var grossCents int64
		var realizedPnL *int64
		if err := rows.Scan(
			&entry.ID,
			&entry.AccountID,
			&entry.IdempotencyKey,
			&entry.Symbol,
			&entry.Side,
			&entry.Type,
			&entry.Quantity,
			&limitPrice,
			&entry.Status,
			&entry.SubmittedAt,
			&entry.Execution.ID,
			&entry.Execution.OrderID,
			&entry.Execution.Symbol,
			&entry.Execution.Quantity,
			&priceCents,
			&entry.Execution.OccurredAt,
			&entry.Execution.QuoteAsOf,
			&entry.Execution.QuoteSource,
			&grossCents,
			&realizedPnL,
			&entry.Execution.CashTransactionID,
		); err != nil {
			return OrderHistoryPage{}, fmt.Errorf("scan paper order: %w", err)
		}
		if limitPrice != nil {
			entry.LimitPriceCents = Money(*limitPrice)
		}
		entry.SubmittedAt = entry.SubmittedAt.UTC()
		entry.Execution.PriceCents = Money(priceCents)
		entry.Execution.OccurredAt = entry.Execution.OccurredAt.UTC()
		entry.Execution.QuoteAsOf = entry.Execution.QuoteAsOf.UTC()
		entry.Execution.GrossCents = Money(grossCents)
		if realizedPnL != nil {
			value := Money(*realizedPnL)
			entry.Execution.RealizedPnLCents = &value
		}
		orders = append(orders, entry)
	}
	if err := rows.Err(); err != nil {
		return OrderHistoryPage{}, fmt.Errorf("iterate paper orders: %w", err)
	}
	return OrderHistoryPage{
		Orders: orders,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
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
