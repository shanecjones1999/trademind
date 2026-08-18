package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shanecjones1999/trademind/internal/market"
)

type stubQuotes struct {
	quote market.Quote
	err   error
}

func (s stubQuotes) Quote(_ context.Context, _ string) (market.Quote, error) {
	return s.quote, s.err
}

type recordingQuotes struct {
	quotes map[string]market.Quote
	calls  []string
}

type stubTickers struct {
	tickers []market.Ticker
	err     error
	search  string
	limit   int
}

func (s *stubTickers) SearchTickers(_ context.Context, search string, limit int) ([]market.Ticker, error) {
	s.search = search
	s.limit = limit
	return s.tickers, s.err
}

func (s *recordingQuotes) Quote(_ context.Context, symbol string) (market.Quote, error) {
	s.calls = append(s.calls, symbol)
	return s.quotes[symbol], nil
}

func TestHealth(t *testing.T) {
	server := NewServer(stubQuotes{}, nil, slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing X-Content-Type-Options header")
	}
}

func TestQuote(t *testing.T) {
	server := NewServer(stubQuotes{
		quote: market.Quote{
			Symbol: "AAPL",
			Price:  191.30,
			AsOf:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			Source: "Massive",
		},
	}, []string{"http://localhost:3000"}, slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/AAPL", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatal("allowed origin was not returned")
	}
}

func TestQuoteListReturnsNormalizedUniqueSymbolsInRequestOrder(t *testing.T) {
	provider := &recordingQuotes{quotes: map[string]market.Quote{
		"AAPL": {Symbol: "AAPL", Price: 191.30},
		"MSFT": {Symbol: "MSFT", Price: 420.18},
	}}
	server := NewServer(provider, nil, slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/quotes?symbols=aapl,%20msft,AAPL", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var quotes []market.Quote
	if err := json.NewDecoder(response.Body).Decode(&quotes); err != nil {
		t.Fatalf("decode quote list: %v", err)
	}
	if len(quotes) != 2 || quotes[0].Symbol != "AAPL" || quotes[1].Symbol != "MSFT" {
		t.Fatalf("quotes = %#v, want AAPL then MSFT", quotes)
	}
	if len(provider.calls) != 2 || provider.calls[0] != "AAPL" || provider.calls[1] != "MSFT" {
		t.Fatalf("provider calls = %#v, want AAPL then MSFT", provider.calls)
	}
}

func TestQuoteListRejectsMissingOrInvalidSymbols(t *testing.T) {
	for _, target := range []string{
		"/api/v1/quotes",
		"/api/v1/quotes?symbols=AAPL,INVALID!",
	} {
		t.Run(target, func(t *testing.T) {
			server := NewServer(stubQuotes{}, nil, slog.Default())
			request := httptest.NewRequest(http.MethodGet, target, nil)
			response := httptest.NewRecorder()

			server.Handler().ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestTickerListSearchesAndLimitsResults(t *testing.T) {
	provider := &stubTickers{tickers: []market.Ticker{
		{Symbol: "AAPL", Name: "Apple Inc."},
	}}
	server := NewServer(stubQuotes{}, nil, slog.Default(), WithTickerProvider(provider))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tickers?search=apple&limit=8", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if provider.search != "apple" || provider.limit != 8 {
		t.Fatalf("ticker request = (%q, %d), want (apple, 8)", provider.search, provider.limit)
	}

	var tickers []market.Ticker
	if err := json.NewDecoder(response.Body).Decode(&tickers); err != nil {
		t.Fatalf("decode ticker list: %v", err)
	}
	if len(tickers) != 1 || tickers[0].Symbol != "AAPL" {
		t.Fatalf("tickers = %#v, want AAPL", tickers)
	}
}

func TestTickerListRejectsInvalidLimit(t *testing.T) {
	server := NewServer(stubQuotes{}, nil, slog.Default(), WithTickerProvider(&stubTickers{}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tickers?limit=13", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestQuoteMapsProviderErrors(t *testing.T) {
	server := NewServer(stubQuotes{err: market.ErrNotConfigured}, nil, slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/AAPL", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestQuoteRejectsUnknownSymbol(t *testing.T) {
	server := NewServer(stubQuotes{err: errors.New("not used")}, nil, slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/AAPL/extra", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCORSPreflightAllowsConfiguredOrigin(t *testing.T) {
	server := NewServer(stubQuotes{}, []string{"http://localhost:3000"}, slog.Default())
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/orders", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	methods := response.Header().Get("Access-Control-Allow-Methods")
	if methods != "GET, POST, OPTIONS" {
		t.Fatalf("allowed methods = %q, want GET, POST, OPTIONS", methods)
	}
}
