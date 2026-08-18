package market

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/0002_quotes.sql
var quoteSchema string

// PostgresQuoteStore serves quotes synced from a market-data provider so the
// API request path never calls the provider directly.
type PostgresQuoteStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func OpenPostgresQuoteStore(ctx context.Context, databaseURL string) (*PostgresQuoteStore, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open quote store connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping quote store database: %w", err)
	}
	if _, err := pool.Exec(ctx, quoteSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply quote store schema: %w", err)
	}
	return &PostgresQuoteStore{pool: pool, now: time.Now}, nil
}

func (s *PostgresQuoteStore) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *PostgresQuoteStore) Quote(ctx context.Context, symbol string) (Quote, error) {
	normalizedSymbol, err := NormalizeSymbol(symbol)
	if err != nil {
		return Quote{}, err
	}

	var quote Quote
	err = s.pool.QueryRow(ctx,
		`SELECT symbol, price, day_change, day_change_pct, as_of, source
		 FROM quotes
		 WHERE symbol = $1`,
		normalizedSymbol,
	).Scan(&quote.Symbol, &quote.Price, &quote.DayChange, &quote.DayChangePct, &quote.AsOf, &quote.Source)
	if errors.Is(err, pgx.ErrNoRows) {
		return Quote{}, ErrSymbolNotFound
	}
	if err != nil {
		return Quote{}, fmt.Errorf("query quote: %w", err)
	}
	return quote, nil
}

func (s *PostgresQuoteStore) Quotes(ctx context.Context, symbols []string) ([]Quote, error) {
	normalizedSymbols := make([]string, 0, len(symbols))
	seen := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		normalizedSymbol, err := NormalizeSymbol(symbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalizedSymbol]; exists {
			continue
		}
		seen[normalizedSymbol] = struct{}{}
		normalizedSymbols = append(normalizedSymbols, normalizedSymbol)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT symbol, price, day_change, day_change_pct, as_of, source
		 FROM quotes
		 WHERE symbol = ANY($1)
		 ORDER BY symbol`,
		normalizedSymbols,
	)
	if err != nil {
		return nil, fmt.Errorf("query quotes: %w", err)
	}
	defer rows.Close()

	quotes := make([]Quote, 0, len(normalizedSymbols))
	for rows.Next() {
		var quote Quote
		if err := rows.Scan(&quote.Symbol, &quote.Price, &quote.DayChange, &quote.DayChangePct, &quote.AsOf, &quote.Source); err != nil {
			return nil, fmt.Errorf("scan quote: %w", err)
		}
		quotes = append(quotes, quote)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quotes: %w", err)
	}
	return quotes, nil
}

// UpsertQuotes replaces the persisted quote for each symbol. It is used by
// the quote-sync job, never by the API request path.
func (s *PostgresQuoteStore) UpsertQuotes(ctx context.Context, quotes []Quote) error {
	if len(quotes) == 0 {
		return nil
	}

	now := s.now().UTC()
	batch := &pgx.Batch{}
	for _, quote := range quotes {
		batch.Queue(
			`INSERT INTO quotes (symbol, price, day_change, day_change_pct, as_of, source, synced_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (symbol) DO UPDATE
			 SET price = EXCLUDED.price,
			     day_change = EXCLUDED.day_change,
			     day_change_pct = EXCLUDED.day_change_pct,
			     as_of = EXCLUDED.as_of,
			     source = EXCLUDED.source,
			     synced_at = EXCLUDED.synced_at`,
			quote.Symbol, quote.Price, quote.DayChange, quote.DayChangePct, quote.AsOf, quote.Source, now,
		)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range quotes {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upsert quote: %w", err)
		}
	}
	return nil
}
