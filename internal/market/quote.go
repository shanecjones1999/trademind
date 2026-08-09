package market

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotConfigured  = errors.New("market data provider is not configured")
	ErrSymbolNotFound = errors.New("symbol not found")
	ErrUnavailable    = errors.New("market data provider is unavailable")

	symbolPattern = regexp.MustCompile(`^[A-Z][A-Z0-9.-]{0,14}$`)
)

type Quote struct {
	Symbol       string    `json:"symbol"`
	Price        float64   `json:"price"`
	Bid          float64   `json:"bid,omitempty"`
	Ask          float64   `json:"ask,omitempty"`
	DayChange    float64   `json:"day_change"`
	DayChangePct float64   `json:"day_change_pct"`
	AsOf         time.Time `json:"as_of"`
	Source       string    `json:"source"`
}

type QuoteProvider interface {
	Quote(ctx context.Context, symbol string) (Quote, error)
}

type BatchQuoteProvider interface {
	Quotes(ctx context.Context, symbols []string) ([]Quote, error)
}

func NormalizeSymbol(symbol string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	if !symbolPattern.MatchString(normalized) {
		return "", ErrSymbolNotFound
	}
	return normalized, nil
}

func IsConfigurationError(err error) bool {
	return errors.Is(err, ErrNotConfigured)
}
