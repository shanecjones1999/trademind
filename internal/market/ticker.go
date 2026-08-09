package market

import "context"

type Ticker struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type TickerProvider interface {
	SearchTickers(ctx context.Context, search string, limit int) ([]Ticker, error)
}
