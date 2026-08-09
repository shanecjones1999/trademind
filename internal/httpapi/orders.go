package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/shanecjones1999/trademind/internal/market"
	"github.com/shanecjones1999/trademind/internal/paper"
)

const maximumDelayedQuoteAge = 7 * 24 * time.Hour

type createOrderRequest struct {
	Side     string `json:"side"`
	Symbol   string `json:"symbol"`
	Quantity int64  `json:"quantity"`
}

type createOrderResponse struct {
	Account paper.AccountSnapshot `json:"account"`
	Fill    paper.OrderFill       `json:"fill"`
}

func (s *Server) orders(writer http.ResponseWriter, request *http.Request) {
	if !s.tradingConfigured(writer) {
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}

	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}

	var body createOrderRequest
	if !decodeRequestJSON(writer, request, &body) {
		return
	}
	if body.Quantity <= 0 {
		writeError(writer, http.StatusBadRequest, "quantity must be a positive whole number")
		return
	}
	side, err := parseOrderSide(body.Side)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "order side must be buy or sell")
		return
	}

	symbol, err := market.NormalizeSymbol(body.Symbol)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "symbol is invalid")
		return
	}

	snapshot, err := s.loadAccountSnapshot(request.Context(), session.Subject)
	if err != nil {
		s.logger.Error("load paper account for order", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "unable to load paper account")
		return
	}

	quote, err := s.quotes.Quote(request.Context(), symbol)
	if err != nil {
		s.writeQuoteError(writer, err)
		return
	}
	executableQuote, err := executableQuoteFromMarketQuote(quote)
	if err != nil {
		s.logger.Error("convert quote for order execution", "error", err, "symbol", symbol)
		writeError(writer, http.StatusBadGateway, "market data is temporarily unavailable")
		return
	}

	now := time.Now().UTC()
	order, err := paper.NewOrder(paper.OrderRequest{
		ID:             newOrderID(),
		IdempotencyKey: newOrderID(),
		AccountID:      snapshot.Account.ID,
		Symbol:         symbol,
		Side:           side,
		Type:           paper.OrderTypeMarket,
		Quantity:       body.Quantity,
	}, now)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	fill, err := paper.FillOrder(
		order,
		executableQuote,
		paper.FillPolicy{
			MaximumQuoteAge: maximumDelayedQuoteAge,
			SlippageBps:     0,
		},
		snapshot.CashBalanceCents,
		availableQuantity(snapshot.Positions, symbol),
		now,
	)
	if err != nil {
		writeOrderError(writer, err)
		return
	}
	if err := s.googleAuth.Accounts.ApplyOrderFill(request.Context(), fill); err != nil {
		s.logger.Error("persist paper order fill", "error", err, "symbol", symbol)
		writeError(writer, http.StatusServiceUnavailable, "unable to place order")
		return
	}

	updatedSnapshot, err := s.loadAccountSnapshot(request.Context(), session.Subject)
	if err != nil {
		s.logger.Error("load updated paper account", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "unable to place order")
		return
	}
	writeJSON(writer, http.StatusCreated, createOrderResponse{
		Account: updatedSnapshot,
		Fill:    fill,
	})
}

func (s *Server) tradingConfigured(writer http.ResponseWriter) bool {
	if !s.authenticationConfigured(writer) {
		return false
	}
	if s.googleAuth.Accounts == nil {
		writeError(writer, http.StatusServiceUnavailable, "paper trading is not configured")
		return false
	}
	return true
}

func writeOrderError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, paper.ErrInsufficientBuyingPower):
		writeError(writer, http.StatusConflict, "insufficient buying power")
	case errors.Is(err, paper.ErrInsufficientPosition):
		writeError(writer, http.StatusConflict, "insufficient position quantity")
	case errors.Is(err, paper.ErrInvalidOrder):
		writeError(writer, http.StatusBadRequest, "order details are invalid")
	case errors.Is(err, paper.ErrStaleQuote), errors.Is(err, paper.ErrLimitNotEligible):
		writeError(writer, http.StatusBadGateway, "market data is temporarily unavailable")
	default:
		writeError(writer, http.StatusServiceUnavailable, "unable to place order")
	}
}

func executableQuoteFromMarketQuote(quote market.Quote) (paper.ExecutableQuote, error) {
	if quote.AsOf.IsZero() || quote.Source == "" {
		return paper.ExecutableQuote{}, fmt.Errorf("quote metadata is incomplete")
	}

	bid := quote.Bid
	if bid <= 0 {
		bid = quote.Price
	}
	ask := quote.Ask
	if ask <= 0 {
		ask = quote.Price
	}
	bidCents, err := dollarsToCents(bid)
	if err != nil {
		return paper.ExecutableQuote{}, err
	}
	askCents, err := dollarsToCents(ask)
	if err != nil {
		return paper.ExecutableQuote{}, err
	}

	return paper.ExecutableQuote{
		BidCents: bidCents,
		AskCents: askCents,
		AsOf:     quote.AsOf.UTC(),
		Source:   quote.Source,
	}, nil
}

func dollarsToCents(value float64) (paper.Money, error) {
	if value <= 0 {
		return 0, fmt.Errorf("price must be positive")
	}
	cents := math.Round(value * 100)
	if cents <= 0 || cents > float64(math.MaxInt64) {
		return 0, fmt.Errorf("price exceeds supported range")
	}
	return paper.Money(cents), nil
}

func parseOrderSide(rawSide string) (paper.OrderSide, error) {
	switch rawSide {
	case "", string(paper.OrderSideBuy):
		return paper.OrderSideBuy, nil
	case string(paper.OrderSideSell):
		return paper.OrderSideSell, nil
	default:
		return "", fmt.Errorf("invalid order side")
	}
}

func availableQuantity(positions []paper.Position, symbol string) int64 {
	for _, position := range positions {
		if position.Symbol == symbol {
			return position.Quantity
		}
	}
	return 0
}

func newOrderID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("order_%d", time.Now().UnixNano())
	}
	return "order_" + hex.EncodeToString(bytes)
}
