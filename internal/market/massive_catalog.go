package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const massiveCatalogBaseURL = "https://api.massive.com"

// MassiveCatalogClient retrieves paginated reference data independently from
// the quote client so catalog synchronization can follow provider cursors.
type MassiveCatalogClient struct {
	apiKey     string
	baseURL    *url.URL
	httpClient *http.Client
}

func NewMassiveCatalogClient(apiKey string) *MassiveCatalogClient {
	client, err := newMassiveCatalogClient(apiKey, massiveCatalogBaseURL, http.DefaultClient)
	if err != nil {
		panic(fmt.Sprintf("configure Massive catalog client: %v", err))
	}
	return client
}

func newMassiveCatalogClient(apiKey, baseURL string, httpClient *http.Client) (*MassiveCatalogClient, error) {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, fmt.Errorf("invalid Massive catalog base URL %q", baseURL)
	}
	if httpClient == nil {
		return nil, fmt.Errorf("Massive catalog HTTP client is required")
	}
	return &MassiveCatalogClient{
		apiKey:     strings.TrimSpace(apiKey),
		baseURL:    parsedBaseURL,
		httpClient: httpClient,
	}, nil
}

func (c *MassiveCatalogClient) ListCatalogPage(ctx context.Context, scope CatalogScope, nextURL string) (CatalogPage, error) {
	if c.apiKey == "" {
		return CatalogPage{}, ErrNotConfigured
	}
	requestURL, err := c.catalogRequestURL(scope, nextURL)
	if err != nil {
		return CatalogPage{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("create Massive ticker request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return CatalogPage{}, fmt.Errorf("%w: request ticker catalog: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CatalogPage{}, fmt.Errorf("%w: Massive returned HTTP %d", ErrUnavailable, response.StatusCode)
	}

	var payload massiveCatalogResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&payload); err != nil {
		return CatalogPage{}, fmt.Errorf("%w: decode ticker catalog: %v", ErrUnavailable, err)
	}
	instruments := make([]CatalogInstrument, 0, len(payload.Results))
	for _, ticker := range payload.Results {
		providerUpdatedAt, err := parseMassiveCatalogTime(ticker.LastUpdatedUTC)
		if err != nil {
			return CatalogPage{}, fmt.Errorf("%w: parse ticker %q update time: %v", ErrUnavailable, ticker.Ticker, err)
		}
		delistedAt, err := parseMassiveCatalogTime(ticker.DelistedUTC)
		if err != nil {
			return CatalogPage{}, fmt.Errorf("%w: parse ticker %q delisting time: %v", ErrUnavailable, ticker.Ticker, err)
		}
		instruments = append(instruments, CatalogInstrument{
			Symbol:            ticker.Ticker,
			Name:              ticker.Name,
			AssetType:         ticker.Type,
			PrimaryExchange:   ticker.PrimaryExchange,
			CompositeFIGI:     ticker.CompositeFIGI,
			ShareClassFIGI:    ticker.ShareClassFIGI,
			Active:            ticker.Active,
			ProviderUpdatedAt: providerUpdatedAt,
			DelistedAt:        delistedAt,
		})
	}
	return CatalogPage{Instruments: instruments, NextURL: payload.NextURL}, nil
}

func (c *MassiveCatalogClient) catalogRequestURL(scope CatalogScope, nextURL string) (*url.URL, error) {
	var requestURL *url.URL
	if nextURL == "" {
		requestURL = c.baseURL.ResolveReference(&url.URL{Path: "/v3/reference/tickers"})
		query := requestURL.Query()
		query.Set("active", "true")
		query.Set("market", "stocks")
		query.Set("exchange", scope.Exchange)
		query.Set("type", scope.AssetType)
		query.Set("sort", "ticker")
		query.Set("order", "asc")
		query.Set("limit", "1000")
		requestURL.RawQuery = query.Encode()
	} else {
		parsed, err := url.Parse(nextURL)
		if err != nil {
			return nil, fmt.Errorf("parse Massive pagination URL: %w", err)
		}
		requestURL = c.baseURL.ResolveReference(parsed)
		if requestURL.Scheme != c.baseURL.Scheme || requestURL.Host != c.baseURL.Host ||
			requestURL.Path != "/v3/reference/tickers" {
			return nil, fmt.Errorf("reject unexpected Massive pagination URL")
		}
	}
	query := requestURL.Query()
	query.Set("apiKey", c.apiKey)
	requestURL.RawQuery = query.Encode()
	return requestURL, nil
}

type massiveCatalogResponse struct {
	NextURL string `json:"next_url"`
	Results []struct {
		Ticker          string `json:"ticker"`
		Name            string `json:"name"`
		Type            string `json:"type"`
		PrimaryExchange string `json:"primary_exchange"`
		CompositeFIGI   string `json:"composite_figi"`
		ShareClassFIGI  string `json:"share_class_figi"`
		Active          bool   `json:"active"`
		LastUpdatedUTC  string `json:"last_updated_utc"`
		DelistedUTC     string `json:"delisted_utc"`
	} `json:"results"`
}

func parseMassiveCatalogTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err == nil {
		parsed = parsed.UTC()
		return &parsed, nil
	}
	parsed, err = time.Parse(time.DateOnly, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
