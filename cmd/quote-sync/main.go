package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/shanecjones1999/trademind/internal/config"
	"github.com/shanecjones1999/trademind/internal/market"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required for quote synchronization")
		os.Exit(1)
	}
	if cfg.MassiveAPIKey == "" {
		logger.Error("MASSIVE_API_KEY is required for quote synchronization")
		os.Exit(1)
	}

	ctx := context.Background()
	store, err := market.OpenPostgresQuoteStore(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("initialize quote store", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	client := market.NewMassiveClient(cfg.MassiveAPIKey)
	date := market.LastCompletedTradingDate()

	quotes, err := client.SyncGroupedDaily(ctx, date)
	if err != nil {
		logger.Error("fetch grouped daily quotes", "date", date, "error", err)
		os.Exit(1)
	}

	if err := store.UpsertQuotes(ctx, quotes); err != nil {
		logger.Error("persist quotes", "date", date, "error", err)
		os.Exit(1)
	}

	logger.Info("synchronized quotes", "date", date, "records", len(quotes))
}
