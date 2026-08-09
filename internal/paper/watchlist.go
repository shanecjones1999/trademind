package paper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrWatchlistNotFound        = errors.New("watchlist not found")
	ErrDuplicateWatchlistSymbol = errors.New("watchlist already contains symbol")
)

type Watchlist struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Name      string            `json:"name"`
	Symbols   []WatchlistSymbol `json:"symbols"`
	CreatedAt time.Time         `json:"created_at"`
}

type WatchlistSymbol struct {
	Symbol  string    `json:"symbol"`
	AddedAt time.Time `json:"added_at"`
}

// WatchlistRepository persists watchlists that belong to a signed-in user.
type WatchlistRepository interface {
	ListWatchlists(ctx context.Context, userID string) ([]Watchlist, error)
	CreateWatchlist(ctx context.Context, userID, name string) (Watchlist, error)
	AddWatchlistSymbol(ctx context.Context, userID, watchlistID, symbol string) (WatchlistSymbol, error)
	RemoveWatchlistSymbol(ctx context.Context, userID, watchlistID, symbol string) error
}

func validateWatchlistName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return "", fmt.Errorf("%w: watchlist names must be between 1 and 80 characters", ErrInvalidTransaction)
	}
	return name, nil
}
