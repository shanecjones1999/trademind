package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/shanecjones1999/trademind/internal/config"
	"github.com/shanecjones1999/trademind/internal/market"
)

func main() {
	scopeName := flag.String("scope", "next", "catalog scope key, or next")
	pages := flag.Int("pages", 1, "maximum provider pages to import")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *pages < 1 || *pages > 100 {
		logger.Error("invalid page limit", "pages", *pages)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required for ticker synchronization")
		os.Exit(1)
	}
	if cfg.MassiveAPIKey == "" {
		logger.Error("MASSIVE_API_KEY is required for ticker synchronization")
		os.Exit(1)
	}

	ctx := context.Background()
	catalog, err := market.OpenPostgresTickerCatalog(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("initialize ticker catalog", "error", err)
		os.Exit(1)
	}
	defer catalog.Close()

	synchronizer, err := market.NewCatalogSynchronizer(market.NewMassiveCatalogClient(cfg.MassiveAPIKey), catalog)
	if err != nil {
		logger.Error("initialize ticker synchronizer", "error", err)
		os.Exit(1)
	}

	scope, err := selectScope(ctx, synchronizer, *scopeName)
	if err != nil {
		logger.Error("select ticker catalog scope", "error", err)
		os.Exit(1)
	}
	for page := 0; page < *pages; page++ {
		result, err := synchronizer.SyncPage(ctx, scope)
		if err != nil {
			logger.Error("synchronize ticker catalog page", "scope", scope.Key, "error", err)
			os.Exit(1)
		}
		logger.Info(
			"synchronized ticker catalog page",
			"scope", result.Scope.Key,
			"run_id", result.RunID,
			"records", result.RecordCount,
			"completed", result.CompletedRun,
		)
		if result.CompletedRun {
			break
		}
	}
}

func selectScope(ctx context.Context, synchronizer *market.CatalogSynchronizer, scopeName string) (market.CatalogScope, error) {
	scopes := market.DefaultCatalogScopes()
	scopeName = strings.ToUpper(strings.TrimSpace(scopeName))
	if scopeName == "" || scopeName == "NEXT" {
		return synchronizer.NextScope(ctx, scopes)
	}
	for _, scope := range scopes {
		if scope.Key == scopeName {
			return scope, nil
		}
	}
	return market.CatalogScope{}, fmt.Errorf("unknown scope %q", scopeName)
}
