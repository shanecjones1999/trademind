package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	massive "github.com/massive-com/client-go/v3/rest"
	"github.com/massive-com/client-go/v3/rest/gen"
)

type MassiveClient struct {
	apiKey          string
	client          massivePreviousDayClient
	tickers         massiveTickerClient
	daily           massiveGroupedDailyClient
	quoteCacheDate  string
	quoteCache      map[string]Quote
	quoteCacheMutex sync.Mutex
}

type massivePreviousDayClient interface {
	PreviousDay(ctx context.Context, symbol string) (massivePreviousDay, error)
}

type massiveTickerClient interface {
	SearchTickers(ctx context.Context, search string, limit int) ([]Ticker, error)
}

type massiveGroupedDailyClient interface {
	GroupedDaily(ctx context.Context, date string) ([]massiveDailyBar, error)
}

type massivePreviousDay struct {
	Symbol string
	Open   float64
	Close  float64
	AtUnix int64
}

type massiveDailyBar struct {
	Symbol string
	Open   float64
	Close  float64
	AtUnix int64
}

type massiveSDKClient struct {
	client *massive.Client
}

func NewMassiveClient(apiKey string) *MassiveClient {
	client := massivePreviousDayClient(nil)
	tickers := massiveTickerClient(nil)
	daily := massiveGroupedDailyClient(nil)
	if apiKey != "" {
		sdkClient := massiveSDKClient{
			client: massive.NewWithOptions(
				apiKey,
				massive.WithTrace(false),
				massive.WithPagination(false),
			),
		}
		client = sdkClient
		tickers = sdkClient
		daily = sdkClient
	}
	return &MassiveClient{
		apiKey:  apiKey,
		client:  client,
		tickers: tickers,
		daily:   daily,
	}
}

func newMassiveClient(apiKey string, client massivePreviousDayClient) *MassiveClient {
	return &MassiveClient{
		apiKey: apiKey,
		client: client,
	}
}

func (c *MassiveClient) Quotes(ctx context.Context, symbols []string) ([]Quote, error) {
	if c.apiKey == "" || c.daily == nil {
		return nil, ErrNotConfigured
	}

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

	date := lastCompletedTradingDate(time.Now()).Format(time.DateOnly)
	c.quoteCacheMutex.Lock()
	defer c.quoteCacheMutex.Unlock()
	if c.quoteCacheDate != date {
		bars, err := c.daily.GroupedDaily(ctx, date)
		if err != nil {
			return nil, err
		}
		c.quoteCache = dailyQuotes(bars)
		c.quoteCacheDate = date
	}

	quotes := make([]Quote, 0, len(normalizedSymbols))
	for _, symbol := range normalizedSymbols {
		if quote, ok := c.quoteCache[symbol]; ok {
			quotes = append(quotes, quote)
		}
	}
	return quotes, nil
}

func (c *MassiveClient) Quote(ctx context.Context, symbol string) (Quote, error) {
	if c.apiKey == "" || c.client == nil {
		return Quote{}, ErrNotConfigured
	}

	normalizedSymbol, err := NormalizeSymbol(symbol)
	if err != nil {
		return Quote{}, err
	}

	bar, err := c.client.PreviousDay(ctx, normalizedSymbol)
	if err != nil {
		return Quote{}, err
	}
	if bar.Close <= 0 {
		return Quote{}, fmt.Errorf("%w: Massive response did not include a usable close price", ErrUnavailable)
	}

	dayChange := bar.Close - bar.Open
	dayChangePct := 0.0
	if bar.Open > 0 {
		dayChangePct = dayChange / bar.Open * 100
	}
	if bar.Symbol == "" {
		bar.Symbol = normalizedSymbol
	}

	return Quote{
		Symbol:       bar.Symbol,
		Price:        bar.Close,
		DayChange:    dayChange,
		DayChangePct: dayChangePct,
		AsOf:         massiveTimestamp(bar.AtUnix),
		Source:       "Massive (end-of-day)",
	}, nil
}

// SyncGroupedDaily fetches every symbol's end-of-day bar for date directly
// from Massive, bypassing the in-memory cache used by Quotes. It is intended
// for a periodic sync job that persists quotes to Postgres so the API
// request path never calls Massive.
func (c *MassiveClient) SyncGroupedDaily(ctx context.Context, date string) ([]Quote, error) {
	if c.apiKey == "" || c.daily == nil {
		return nil, ErrNotConfigured
	}

	bars, err := c.daily.GroupedDaily(ctx, date)
	if err != nil {
		return nil, err
	}

	quotesBySymbol := dailyQuotes(bars)
	quotes := make([]Quote, 0, len(quotesBySymbol))
	for _, quote := range quotesBySymbol {
		quotes = append(quotes, quote)
	}
	return quotes, nil
}

// LastCompletedTradingDate returns the most recent completed trading date
// (America/New_York, skipping weekends) formatted as YYYY-MM-DD.
func LastCompletedTradingDate() string {
	return lastCompletedTradingDate(time.Now()).Format(time.DateOnly)
}

func (c *MassiveClient) SearchTickers(ctx context.Context, search string, limit int) ([]Ticker, error) {
	if c.apiKey == "" || c.tickers == nil {
		return nil, ErrNotConfigured
	}
	return c.tickers.SearchTickers(ctx, search, limit)
}

func (c massiveSDKClient) PreviousDay(ctx context.Context, symbol string) (massivePreviousDay, error) {
	response, err := c.client.GetPreviousStocksAggregates(
		ctx,
		symbol,
		&gen.GetPreviousStocksAggregatesParams{Adjusted: massive.Ptr(true)},
	)
	if err != nil {
		return massivePreviousDay{}, fmt.Errorf("%w: request previous-day aggregate: %v", ErrUnavailable, err)
	}
	if response == nil {
		return massivePreviousDay{}, fmt.Errorf("%w: Massive returned no HTTP response", ErrUnavailable)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return massivePreviousDay{}, ErrSymbolNotFound
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return massivePreviousDay{}, fmt.Errorf("%w: Massive returned HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var payload massivePreviousDayResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return massivePreviousDay{}, fmt.Errorf("%w: decode previous-day aggregate: %v", ErrUnavailable, err)
	}
	if len(payload.Results) == 0 {
		return massivePreviousDay{}, ErrSymbolNotFound
	}

	bar := payload.Results[0]
	timestamp, err := parseMassiveTimestamp(bar.Timestamp)
	if err != nil {
		return massivePreviousDay{}, fmt.Errorf("%w: parse aggregate timestamp: %v", ErrUnavailable, err)
	}
	return massivePreviousDay{
		Symbol: payload.Ticker,
		Open:   bar.Open,
		Close:  bar.Close,
		AtUnix: timestamp,
	}, nil
}

func (c massiveSDKClient) GroupedDaily(ctx context.Context, date string) ([]massiveDailyBar, error) {
	response, err := c.client.GetGroupedStocksAggregates(
		ctx,
		date,
		&gen.GetGroupedStocksAggregatesParams{Adjusted: massive.Ptr(true)},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: request grouped daily aggregates: %v", ErrUnavailable, err)
	}
	if response == nil {
		return nil, fmt.Errorf("%w: Massive returned no HTTP response", ErrUnavailable)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: Massive returned HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var payload massiveGroupedDailyResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode grouped daily aggregates: %v", ErrUnavailable, err)
	}

	bars := make([]massiveDailyBar, 0, len(payload.Results))
	for _, bar := range payload.Results {
		if bar.Symbol == "" || bar.Close <= 0 {
			continue
		}
		bars = append(bars, massiveDailyBar{
			Symbol: bar.Symbol,
			Open:   bar.Open,
			Close:  bar.Close,
			AtUnix: bar.Timestamp,
		})
	}
	return bars, nil
}

func (c massiveSDKClient) SearchTickers(ctx context.Context, search string, limit int) ([]Ticker, error) {
	response, err := c.client.ListTickers(ctx, &gen.ListTickersParams{
		Active: massive.Ptr(true),
		Limit:  massive.Ptr(limit),
		Market: massive.Ptr(gen.ListTickersParamsMarketStocks),
		Search: massive.Ptr(search),
		Sort:   massive.Ptr(gen.ListTickersParamsSortTicker),
		Type:   massive.Ptr(gen.ListTickersParamsTypeCS),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: request ticker list: %v", ErrUnavailable, err)
	}
	if response == nil {
		return nil, fmt.Errorf("%w: Massive returned no HTTP response", ErrUnavailable)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: Massive returned HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var payload massiveTickersResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: decode ticker list: %v", ErrUnavailable, err)
	}

	tickers := make([]Ticker, 0, len(payload.Results))
	for _, ticker := range payload.Results {
		if ticker.Ticker == "" || ticker.Name == "" {
			continue
		}
		tickers = append(tickers, Ticker{
			Symbol: ticker.Ticker,
			Name:   ticker.Name,
		})
	}
	return tickers, nil
}

type massivePreviousDayResponse struct {
	Ticker  string `json:"ticker"`
	Results []struct {
		Open      float64         `json:"o"`
		Close     float64         `json:"c"`
		Timestamp json.RawMessage `json:"t"`
	} `json:"results"`
}

type massiveTickersResponse struct {
	Results []struct {
		Ticker string `json:"ticker"`
		Name   string `json:"name"`
	} `json:"results"`
}

type massiveGroupedDailyResponse struct {
	Results []struct {
		Symbol    string  `json:"T"`
		Open      float64 `json:"o"`
		Close     float64 `json:"c"`
		Timestamp int64   `json:"t"`
	} `json:"results"`
}

func dailyQuotes(bars []massiveDailyBar) map[string]Quote {
	quotes := make(map[string]Quote, len(bars))
	for _, bar := range bars {
		if bar.Symbol == "" || bar.Close <= 0 {
			continue
		}
		dayChange := bar.Close - bar.Open
		dayChangePct := 0.0
		if bar.Open > 0 {
			dayChangePct = dayChange / bar.Open * 100
		}
		quotes[bar.Symbol] = Quote{
			Symbol:       bar.Symbol,
			Price:        bar.Close,
			DayChange:    dayChange,
			DayChangePct: dayChangePct,
			AsOf:         massiveTimestamp(bar.AtUnix),
			Source:       "Massive (end-of-day)",
		}
	}
	return quotes
}

func lastCompletedTradingDate(now time.Time) time.Time {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		location = time.UTC
	}
	date := now.In(location).AddDate(0, 0, -1)
	for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		date = date.AddDate(0, 0, -1)
	}
	return date
}

func parseMassiveTimestamp(raw json.RawMessage) (int64, error) {
	value := strings.Trim(string(raw), `"`)
	timestamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return timestamp, nil
}

func massiveTimestamp(timestamp int64) time.Time {
	switch {
	case timestamp >= 100_000_000_000_000_000:
		return time.Unix(0, timestamp).UTC()
	case timestamp >= 100_000_000_000_000:
		return time.UnixMicro(timestamp).UTC()
	case timestamp >= 100_000_000_000:
		return time.UnixMilli(timestamp).UTC()
	default:
		return time.Unix(timestamp, 0).UTC()
	}
}
