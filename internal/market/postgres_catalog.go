package market

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
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/0001_instruments.sql
var instrumentSchema string

// PostgresTickerCatalog persists locally searchable instrument reference data
// and the cursor required to resume a Massive import.
type PostgresTickerCatalog struct {
	pool  *pgxpool.Pool
	now   func() time.Time
	newID func() (string, error)
}

func OpenPostgresTickerCatalog(ctx context.Context, databaseURL string) (*PostgresTickerCatalog, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open ticker catalog connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping ticker catalog database: %w", err)
	}
	if _, err := pool.Exec(ctx, instrumentSchema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("apply instrument catalog schema: %w", err)
	}
	return &PostgresTickerCatalog{
		pool:  pool,
		now:   time.Now,
		newID: newCatalogRunID,
	}, nil
}

func (c *PostgresTickerCatalog) Close() {
	if c != nil && c.pool != nil {
		c.pool.Close()
	}
}

func (c *PostgresTickerCatalog) SearchTickers(ctx context.Context, search string, limit int) ([]Ticker, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("ticker search limit must be between 1 and 100")
	}
	search = strings.TrimSpace(search)
	prefix := strings.ToUpper(search)
	rows, err := c.pool.Query(ctx,
		`SELECT symbol, name
		 FROM instruments
		 WHERE active
		   AND ($1 = '' OR symbol ILIKE $1 || '%' OR name ILIKE '%' || $1 || '%')
		 ORDER BY
		   CASE WHEN symbol = $2 THEN 0 WHEN symbol ILIKE $1 || '%' THEN 1 ELSE 2 END,
		   symbol ASC
		 LIMIT $3`,
		search, prefix, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search ticker catalog: %w", err)
	}
	defer rows.Close()

	tickers := make([]Ticker, 0, limit)
	for rows.Next() {
		var ticker Ticker
		if err := rows.Scan(&ticker.Symbol, &ticker.Name); err != nil {
			return nil, fmt.Errorf("scan ticker catalog result: %w", err)
		}
		tickers = append(tickers, ticker)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticker catalog results: %w", err)
	}
	return tickers, nil
}

func (c *PostgresTickerCatalog) ScopeState(ctx context.Context, scope CatalogScope) (CatalogScopeState, error) {
	var state CatalogScopeState
	err := c.pool.QueryRow(ctx,
		`SELECT current_run_id, next_url, status
		 FROM instrument_sync_scopes
		 WHERE scope_key = $1`,
		scope.Key,
	).Scan(&state.Run.ID, &state.Run.NextURL, &state.Run.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return CatalogScopeState{}, fmt.Errorf("query catalog scope: %w", err)
	}
	state.Exists = true
	return state, nil
}

func (c *PostgresTickerCatalog) StartOrResumeCatalogRun(ctx context.Context, scope CatalogScope) (CatalogRun, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return CatalogRun{}, fmt.Errorf("begin catalog run: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var run CatalogRun
	err = tx.QueryRow(ctx,
		`SELECT current_run_id, next_url, status
		 FROM instrument_sync_scopes
		 WHERE scope_key = $1
		 FOR UPDATE`,
		scope.Key,
	).Scan(&run.ID, &run.NextURL, &run.Status)
	switch {
	case err == nil && run.Status == "running":
		if err := tx.Commit(ctx); err != nil {
			return CatalogRun{}, fmt.Errorf("commit catalog run lookup: %w", err)
		}
		return run, nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return CatalogRun{}, fmt.Errorf("load catalog run: %w", err)
	}

	run.ID, err = c.newID()
	if err != nil {
		return CatalogRun{}, fmt.Errorf("generate catalog run ID: %w", err)
	}
	run.NextURL = ""
	run.Status = "running"
	now := c.now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO instrument_sync_scopes (
		    scope_key, exchange, asset_type, current_run_id, next_url, status, started_at, updated_at, last_completed_at
		 )
		 VALUES ($1, $2, $3, $4, '', 'running', $5, $5, NULL)
		 ON CONFLICT (scope_key) DO UPDATE
		 SET exchange = EXCLUDED.exchange,
		     asset_type = EXCLUDED.asset_type,
		     current_run_id = EXCLUDED.current_run_id,
		     next_url = '',
		     status = 'running',
		     started_at = EXCLUDED.started_at,
		     updated_at = EXCLUDED.updated_at`,
		scope.Key, scope.Exchange, scope.AssetType, run.ID, now,
	); err != nil {
		return CatalogRun{}, fmt.Errorf("start catalog scope: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO instrument_sync_runs (id, scope_key, status, started_at)
		 VALUES ($1, $2, 'running', $3)`,
		run.ID, scope.Key, now,
	); err != nil {
		return CatalogRun{}, fmt.Errorf("insert catalog run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CatalogRun{}, fmt.Errorf("commit catalog run start: %w", err)
	}
	return run, nil
}

func (c *PostgresTickerCatalog) ApplyCatalogPage(
	ctx context.Context,
	scope CatalogScope,
	run CatalogRun,
	instruments []CatalogInstrument,
	nextURL string,
) (CatalogRun, error) {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return CatalogRun{}, fmt.Errorf("begin catalog page: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentRunID, status string
	if err := tx.QueryRow(ctx,
		`SELECT current_run_id, status
		 FROM instrument_sync_scopes
		 WHERE scope_key = $1
		 FOR UPDATE`,
		scope.Key,
	).Scan(&currentRunID, &status); err != nil {
		return CatalogRun{}, fmt.Errorf("lock catalog scope: %w", err)
	}
	if currentRunID != run.ID || status != "running" {
		return CatalogRun{}, fmt.Errorf("catalog run %q is no longer active for scope %q", run.ID, scope.Key)
	}

	now := c.now().UTC()
	if len(instruments) > 0 {
		batch := &pgx.Batch{}
		for _, instrument := range instruments {
			providerKey := instrument.providerKey(scope)
			batch.Queue(
				`UPDATE instruments
				 SET active = false,
				     delisted_at = COALESCE(delisted_at, $4)
				 WHERE scope_key = $1
				   AND symbol = $2
				   AND provider_key <> $3
				   AND active`,
				scope.Key, instrument.Symbol, providerKey, now,
			)
			batch.Queue(
				`INSERT INTO instruments (
				    provider_key, scope_key, symbol, name, asset_type, primary_exchange,
				    composite_figi, share_class_figi, active, provider_updated_at,
				    first_seen_at, last_seen_at, last_seen_run_id, delisted_at
				 )
				 VALUES (
				    $1, $2, $3, $4, $5, $6, $7::text, $8::text, $9, $10::timestamptz, $11::timestamptz, $11::timestamptz, $12,
				    CASE WHEN $9 THEN NULL ELSE COALESCE($13::timestamptz, $11::timestamptz) END
				 )
				 ON CONFLICT (provider_key) DO UPDATE
				 SET scope_key = EXCLUDED.scope_key,
				     symbol = EXCLUDED.symbol,
				     name = EXCLUDED.name,
				     asset_type = EXCLUDED.asset_type,
				     primary_exchange = EXCLUDED.primary_exchange,
				     composite_figi = EXCLUDED.composite_figi,
				     share_class_figi = EXCLUDED.share_class_figi,
				     active = EXCLUDED.active,
				     provider_updated_at = EXCLUDED.provider_updated_at,
				     last_seen_at = EXCLUDED.last_seen_at,
				     last_seen_run_id = EXCLUDED.last_seen_run_id,
				     delisted_at = CASE
				        WHEN EXCLUDED.active THEN NULL
				        ELSE COALESCE(EXCLUDED.delisted_at, instruments.delisted_at)
				     END`,
				providerKey,
				scope.Key,
				instrument.Symbol,
				instrument.Name,
				instrument.AssetType,
				instrument.PrimaryExchange,
				nullableString(instrument.CompositeFIGI),
				nullableString(instrument.ShareClassFIGI),
				instrument.Active,
				instrument.ProviderUpdatedAt,
				now,
				run.ID,
				instrument.DelistedAt,
			)
		}
		results := tx.SendBatch(ctx, batch)
		for range instruments {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return CatalogRun{}, fmt.Errorf("deactivate replaced catalog listing: %w", err)
			}
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return CatalogRun{}, fmt.Errorf("upsert catalog instrument: %w", err)
			}
		}
		if err := results.Close(); err != nil {
			return CatalogRun{}, fmt.Errorf("execute catalog page batch: %w", err)
		}
	}

	completed := strings.TrimSpace(nextURL) == ""
	if completed {
		if _, err := tx.Exec(ctx,
			`UPDATE instruments
			 SET active = false,
			     delisted_at = COALESCE(delisted_at, $3)
			 WHERE scope_key = $1
			   AND active
			   AND last_seen_run_id <> $2`,
			scope.Key, run.ID, now,
		); err != nil {
			return CatalogRun{}, fmt.Errorf("deactivate unseen catalog instruments: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE instrument_sync_runs
		 SET status = CASE WHEN $2 THEN 'completed' ELSE 'running' END,
		     completed_at = CASE WHEN $2 THEN $3::timestamptz ELSE NULL END,
		     page_count = page_count + 1,
		     record_count = record_count + $4
		 WHERE id = $1`,
		run.ID, completed, now, len(instruments),
	); err != nil {
		return CatalogRun{}, fmt.Errorf("update catalog run: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE instrument_sync_scopes
		 SET next_url = $2,
		     status = CASE WHEN $3 THEN 'completed' ELSE 'running' END,
		     updated_at = $4,
		     last_completed_at = CASE WHEN $3 THEN $4 ELSE last_completed_at END
		 WHERE scope_key = $1`,
		scope.Key, nextURL, completed, now,
	); err != nil {
		return CatalogRun{}, fmt.Errorf("update catalog scope: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CatalogRun{}, fmt.Errorf("commit catalog page: %w", err)
	}
	run.NextURL = nextURL
	if completed {
		run.Status = "completed"
	}
	return run, nil
}

func newCatalogRunID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return "instrument-sync-" + hex.EncodeToString(bytes[:]), nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
