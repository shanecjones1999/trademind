package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shanecjones1999/trademind/internal/config"
	"github.com/shanecjones1999/trademind/internal/httpapi"
	"github.com/shanecjones1999/trademind/internal/identity"
	"github.com/shanecjones1999/trademind/internal/market"
	"github.com/shanecjones1999/trademind/internal/paper"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	quotes := market.NewMassiveClient(cfg.MassiveAPIKey)
	var accounts paper.AccountRepository
	var tickers market.TickerProvider
	if cfg.DatabaseURL != "" {
		store, err := paper.OpenPostgresStore(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("initialize paper-account store", "error", err)
			os.Exit(1)
		}
		defer store.Close()
		accounts = store
		logger.Info("paper-account persistence enabled")

		catalog, err := market.OpenPostgresTickerCatalog(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("initialize ticker catalog", "error", err)
			os.Exit(1)
		}
		defer catalog.Close()
		tickers = catalog
		logger.Info("local ticker catalog enabled")
	} else {
		logger.Warn("paper-account persistence disabled: set DATABASE_URL to provision paper accounts")
		logger.Warn("local ticker catalog disabled: set DATABASE_URL and run ticker-sync before ticker search is available")
	}

	apiOptions := []httpapi.Option{httpapi.WithTickerProvider(tickers)}
	if cfg.GoogleAuthEnabled() {
		sessions, err := identity.NewSessionManager(cfg.AuthSessionSecret)
		if err != nil {
			logger.Error("create session manager", "error", err)
			os.Exit(1)
		}

		googleAuth, err := identity.NewGoogleOIDCAuthenticator(context.Background(), identity.GoogleConfig{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
		})
		if err != nil {
			logger.Error("initialize Google OpenID Connect", "error", err)
			os.Exit(1)
		}

		apiOptions = append(apiOptions, httpapi.WithGoogleAuth(httpapi.GoogleAuthConfig{
			Authenticator:   googleAuth,
			Sessions:        sessions,
			Accounts:        accounts,
			Watchlists:      watchlistRepository(accounts),
			SuccessRedirect: cfg.AppWebURL,
			SecureCookies:   strings.HasPrefix(cfg.GoogleRedirectURL, "https://"),
		}))
		logger.Info("Google authentication enabled")
	} else {
		logger.Warn("Google authentication disabled: set Google OAuth and session configuration to enable it")
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewServer(quotes, cfg.AllowedOrigins, logger, apiOptions...).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("API server listening", "address", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("API server stopped", "error", err)
			os.Exit(1)
		}
	case signal := <-shutdownSignal():
		logger.Info("shutting down API server", "signal", signal.String())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("shutdown API server", "error", err)
			os.Exit(1)
		}
	}
}

func watchlistRepository(accounts paper.AccountRepository) paper.WatchlistRepository {
	watchlists, _ := accounts.(paper.WatchlistRepository)
	return watchlists
}

func shutdownSignal() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	return signals
}
