package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/shanecjones1999/trademind/internal/identity"
	"github.com/shanecjones1999/trademind/internal/market"
	"github.com/shanecjones1999/trademind/internal/paper"
)

const (
	quotesPath             = "/api/v1/quotes"
	quotePathPrefix        = "/api/v1/quotes/"
	tickersPath            = "/api/v1/tickers"
	googleAuthStartPath    = "/api/v1/auth/google"
	googleAuthCallbackPath = "/api/v1/auth/google/callback"
	accountPath            = "/api/v1/account"
	accountActivityPath    = "/api/v1/account/activity"
	ordersPath             = "/api/v1/orders"
	sessionCookieName      = "trademind_session"
	stateCookieName        = "trademind_oauth_state"
	nextCookieName         = "trademind_auth_next"
	maxJSONRequestBytes    = 4 << 10
)

type Server struct {
	quotes         market.QuoteProvider
	tickers        market.TickerProvider
	allowedOrigins map[string]struct{}
	logger         *slog.Logger
	googleAuth     *GoogleAuthConfig
}

type Option func(*Server)

func WithTickerProvider(tickers market.TickerProvider) Option {
	return func(server *Server) {
		server.tickers = tickers
	}
}

type GoogleAuthConfig struct {
	Authenticator   identity.GoogleAuthenticator
	Sessions        *identity.SessionManager
	Accounts        paper.AccountRepository
	SuccessRedirect string
	SecureCookies   bool
}

func WithGoogleAuth(config GoogleAuthConfig) Option {
	return func(server *Server) {
		server.googleAuth = &config
	}
}

func NewServer(quotes market.QuoteProvider, allowedOrigins []string, logger *slog.Logger, options ...Option) *Server {
	origins := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		origins[origin] = struct{}{}
	}

	if logger == nil {
		logger = slog.Default()
	}

	server := &Server{
		quotes:         quotes,
		allowedOrigins: origins,
		logger:         logger,
	}
	for _, option := range options {
		option(server)
	}
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc(quotesPath, s.quoteList)
	mux.HandleFunc(quotePathPrefix, s.quote)
	mux.HandleFunc(tickersPath, s.tickerList)
	mux.HandleFunc(googleAuthStartPath, s.googleAuthStart)
	mux.HandleFunc(googleAuthCallbackPath, s.googleAuthCallback)
	mux.HandleFunc("/api/v1/me", s.me)
	mux.HandleFunc(accountPath, s.account)
	mux.HandleFunc(accountActivityPath, s.activity)
	mux.HandleFunc(ordersPath, s.orders)
	mux.HandleFunc("/api/v1/auth/logout", s.logout)
	return s.securityHeaders(s.cors(s.recoverPanics(mux)))
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *Server) tickerList(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if s.tickers == nil {
		s.logger.Error("ticker provider is not configured")
		writeError(writer, http.StatusServiceUnavailable, "market data is not configured")
		return
	}

	search := strings.TrimSpace(request.URL.Query().Get("search"))
	if len(search) > 80 {
		writeError(writer, http.StatusBadRequest, "search must be 80 characters or fewer")
		return
	}

	limit, err := parseTickerLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "limit must be between 1 and 12")
		return
	}

	tickers, err := s.tickers.SearchTickers(request.Context(), search, limit)
	if err != nil {
		s.writeQuoteError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, tickers)
}

func parseTickerLimit(rawLimit string) (int, error) {
	if rawLimit == "" {
		return 12, nil
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil || limit < 1 || limit > 12 {
		return 0, market.ErrSymbolNotFound
	}
	return limit, nil
}

func (s *Server) quoteList(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}

	symbols, err := parseQuoteListSymbols(request.URL.Query().Get("symbols"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "symbols must be a comma-separated list of up to 12 valid symbols")
		return
	}

	if batchQuotes, ok := s.quotes.(market.BatchQuoteProvider); ok {
		quotes, err := batchQuotes.Quotes(request.Context(), symbols)
		if err != nil {
			s.writeQuoteError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, quotesInRequestedOrder(symbols, quotes))
		return
	}

	quotes := make([]market.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		quote, err := s.quotes.Quote(request.Context(), symbol)
		if err != nil {
			s.writeQuoteError(writer, err)
			return
		}
		quotes = append(quotes, quote)
	}

	writeJSON(writer, http.StatusOK, quotes)
}

func quotesInRequestedOrder(symbols []string, quotes []market.Quote) []market.Quote {
	bySymbol := make(map[string]market.Quote, len(quotes))
	for _, quote := range quotes {
		bySymbol[quote.Symbol] = quote
	}
	ordered := make([]market.Quote, 0, len(symbols))
	for _, symbol := range symbols {
		if quote, ok := bySymbol[symbol]; ok {
			ordered = append(ordered, quote)
		}
	}
	return ordered
}

func (s *Server) quote(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}

	symbol := strings.TrimPrefix(request.URL.Path, quotePathPrefix)
	if symbol == "" || strings.Contains(symbol, "/") {
		writeError(writer, http.StatusNotFound, "quote not found")
		return
	}

	quote, err := s.quotes.Quote(request.Context(), symbol)
	if err != nil {
		s.writeQuoteError(writer, err)
		return
	}

	writeJSON(writer, http.StatusOK, quote)
}

func parseQuoteListSymbols(rawSymbols string) ([]string, error) {
	rawSymbols = strings.TrimSpace(rawSymbols)
	if rawSymbols == "" {
		return nil, market.ErrSymbolNotFound
	}

	symbols := make([]string, 0, 12)
	seen := make(map[string]struct{}, 12)
	for _, rawSymbol := range strings.Split(rawSymbols, ",") {
		symbol, err := market.NormalizeSymbol(rawSymbol)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		if len(symbols) == cap(symbols) {
			return nil, market.ErrSymbolNotFound
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	return symbols, nil
}

func (s *Server) writeQuoteError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, market.ErrSymbolNotFound):
		writeError(writer, http.StatusNotFound, "symbol not found")
	case market.IsConfigurationError(err):
		s.logger.Error("market data provider is not configured")
		writeError(writer, http.StatusServiceUnavailable, "market data is not configured")
	case errors.Is(err, market.ErrUnavailable):
		s.logger.Error("market data request failed", "error", err)
		writeError(writer, http.StatusBadGateway, "market data is temporarily unavailable")
	default:
		s.logger.Error("quote request failed", "error", err)
		writeError(writer, http.StatusInternalServerError, "unable to retrieve quote")
	}
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin != "" {
			if _, allowed := s.allowedOrigins[origin]; allowed {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Vary", "Origin")
				writer.Header().Set("Access-Control-Allow-Methods", http.MethodGet+", "+http.MethodPost+", "+http.MethodOptions)
				writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		if request.Method == http.MethodOptions {
			if origin == "" {
				writeError(writer, http.StatusForbidden, "origin is required")
				return
			}
			if _, allowed := s.allowedOrigins[origin]; !allowed {
				writeError(writer, http.StatusForbidden, "origin is not allowed")
				return
			}
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(writer, request)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("request panic", "recovered", recovered)
				writeError(writer, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func methodNotAllowed(writer http.ResponseWriter, allowed string) {
	writer.Header().Set("Allow", allowed)
	writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{
		"error": message,
	})
}

func decodeRequestJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxJSONRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(writer, http.StatusBadRequest, "request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(writer, http.StatusBadRequest, "request body must contain one JSON object")
		return false
	}
	return true
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(payload); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}
