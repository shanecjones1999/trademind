package market

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

type fakeMassivePreviousDayClient struct {
	bar    massivePreviousDay
	err    error
	symbol string
}

type fakeMassiveGroupedDailyClient struct {
	bars  []massiveDailyBar
	err   error
	calls int
}

func (f *fakeMassiveGroupedDailyClient) GroupedDaily(_ context.Context, _ string) ([]massiveDailyBar, error) {
	f.calls++
	return f.bars, f.err
}

func (f *fakeMassivePreviousDayClient) PreviousDay(_ context.Context, symbol string) (massivePreviousDay, error) {
	f.symbol = symbol
	return f.bar, f.err
}

func TestMassiveClientQuoteNormalizesPreviousDayBar(t *testing.T) {
	sdkClient := &fakeMassivePreviousDayClient{
		bar: massivePreviousDay{
			Symbol: "AAPL",
			Open:   190.06,
			Close:  191.30,
			AtUnix: 1717113600000,
		},
	}
	client := newMassiveClient("test-key", sdkClient)

	quote, err := client.Quote(context.Background(), "aapl")
	if err != nil {
		t.Fatalf("get quote: %v", err)
	}
	if sdkClient.symbol != "AAPL" {
		t.Fatalf("SDK symbol = %q, want AAPL", sdkClient.symbol)
	}

	if quote.Symbol != "AAPL" || quote.Price != 191.30 {
		t.Fatalf("unexpected quote: %#v", quote)
	}
	if math.Abs(quote.DayChange-1.24) > 0.0000000001 {
		t.Fatalf("day change = %f", quote.DayChange)
	}
	if math.Abs(quote.DayChangePct-0.6524255498263706) > 0.0000000001 {
		t.Fatalf("day change percentage = %f", quote.DayChangePct)
	}
	if want := time.UnixMilli(1717113600000).UTC(); !quote.AsOf.Equal(want) {
		t.Fatalf("quote timestamp = %s, want %s", quote.AsOf, want)
	}
	if quote.Source != "Massive (end-of-day)" {
		t.Fatalf("quote source = %q", quote.Source)
	}
}

func TestMassiveClientQuoteRequiresAPIKey(t *testing.T) {
	client := NewMassiveClient("")

	_, err := client.Quote(context.Background(), "AAPL")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Quote error = %v, want ErrNotConfigured", err)
	}
}

func TestMassiveClientQuotesUsesOneGroupedDailyRequest(t *testing.T) {
	dailyClient := &fakeMassiveGroupedDailyClient{bars: []massiveDailyBar{
		{Symbol: "AAPL", Open: 190.06, Close: 191.30, AtUnix: 1717113600000},
		{Symbol: "MSFT", Open: 420.00, Close: 418.00, AtUnix: 1717113600000},
	}}
	client := &MassiveClient{apiKey: "test-key", daily: dailyClient}

	quotes, err := client.Quotes(context.Background(), []string{"aapl", "MSFT", "AAPL"})
	if err != nil {
		t.Fatalf("get quotes: %v", err)
	}
	if dailyClient.calls != 1 {
		t.Fatalf("grouped daily calls = %d, want 1", dailyClient.calls)
	}
	if len(quotes) != 2 || quotes[0].Symbol != "AAPL" || quotes[1].Symbol != "MSFT" {
		t.Fatalf("quotes = %#v, want AAPL then MSFT", quotes)
	}

	_, err = client.Quotes(context.Background(), []string{"AAPL"})
	if err != nil {
		t.Fatalf("get cached quote: %v", err)
	}
	if dailyClient.calls != 1 {
		t.Fatalf("grouped daily calls = %d after cache, want 1", dailyClient.calls)
	}
}

func TestLastCompletedTradingDateSkipsWeekends(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	if got, want := lastCompletedTradingDate(now).Format(time.DateOnly), "2026-08-07"; got != want {
		t.Fatalf("last completed trading date = %s, want %s", got, want)
	}
}

func TestNormalizeSymbolRejectsInvalidSymbols(t *testing.T) {
	for _, symbol := range []string{"", "AAPL/", "$SPX", "AAPL!"} {
		t.Run(symbol, func(t *testing.T) {
			_, err := NormalizeSymbol(symbol)
			if !errors.Is(err, ErrSymbolNotFound) {
				t.Fatalf("NormalizeSymbol(%q) error = %v, want ErrSymbolNotFound", symbol, err)
			}
		})
	}
}

func TestParseMassiveTimestampAcceptsNumberAndString(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`1717113600000`),
		json.RawMessage(`"1717113600000"`),
	} {
		timestamp, err := parseMassiveTimestamp(raw)
		if err != nil {
			t.Fatalf("parse timestamp %s: %v", raw, err)
		}
		if timestamp != 1717113600000 {
			t.Fatalf("timestamp = %d", timestamp)
		}
	}
}
