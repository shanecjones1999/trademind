package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shanecjones1999/trademind/internal/identity"
	"github.com/shanecjones1999/trademind/internal/market"
	"github.com/shanecjones1999/trademind/internal/paper"
)

type fakeGoogleAuthenticator struct {
	state   string
	nonce   string
	profile identity.Profile
	err     error
}

type fakeAccounts struct {
	account       paper.PaperAccount
	snapshot      paper.AccountSnapshot
	ensureErr     error
	snapshotErr   error
	snapshotErrs  []error
	applyErr      error
	appliedFill   paper.OrderFill
	ensuredFor    string
	snapshotFor   []string
	activity      []paper.LedgerActivityEntry
	activityErr   error
	activityFor   string
	activityLimit int
	orders        []paper.OrderHistoryEntry
	ordersErr     error
	ordersFor     string
	ordersLimit   int
	ordersOffset  int
}

func (f *fakeAccounts) EnsureAccount(_ context.Context, userID string) (paper.PaperAccount, error) {
	f.ensuredFor = userID
	return f.account, f.ensureErr
}

func TestOrdersRequireAuthenticationAndFillPaperOrders(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	accounts := &fakeAccounts{
		snapshot: paper.AccountSnapshot{
			Account: paper.PaperAccount{
				ID:       "paper-account-1",
				UserID:   "google-subject",
				OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
			},
			CashBalanceCents: paper.DefaultStartingCashCents,
			Positions:        []paper.Position{},
		},
	}
	server := NewServer(
		stubQuotes{
			quote: market.Quote{
				Symbol: "AAPL",
				Price:  191.30,
				AsOf:   time.Now().UTC().Add(-time.Hour),
				Source: "Massive",
			},
		},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Accounts:      accounts,
		}),
	)

	unauthenticatedRequest := httptest.NewRequest(http.MethodPost, ordersPath, strings.NewReader(`{"symbol":"AAPL","quantity":1}`))
	unauthenticatedRequest.Header.Set("Content-Type", "application/json")
	unauthenticatedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthenticatedResponse, unauthenticatedRequest)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticatedResponse.Code, http.StatusUnauthorized)
	}

	sessionToken, _, err := sessions.CreateSession(identity.Profile{
		Subject: "google-subject",
		Email:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	orderRequest := httptest.NewRequest(http.MethodPost, ordersPath, strings.NewReader(`{"symbol":"aapl","quantity":2}`))
	orderRequest.Header.Set("Content-Type", "application/json")
	orderRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	orderResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(orderResponse, orderRequest)
	if orderResponse.Code != http.StatusCreated {
		t.Fatalf("order status = %d, want %d", orderResponse.Code, http.StatusCreated)
	}
	if accounts.appliedFill.Order.Symbol != "AAPL" {
		t.Fatalf("applied symbol = %q, want AAPL", accounts.appliedFill.Order.Symbol)
	}
	if accounts.appliedFill.Order.Quantity != 2 {
		t.Fatalf("applied quantity = %d, want 2", accounts.appliedFill.Order.Quantity)
	}
	if len(accounts.snapshot.Positions) != 1 || accounts.snapshot.Positions[0].Quantity != 2 {
		t.Fatalf("positions = %#v, want one 2-share position", accounts.snapshot.Positions)
	}

	sellRequest := httptest.NewRequest(http.MethodPost, ordersPath, strings.NewReader(`{"side":"sell","symbol":"aapl","quantity":1}`))
	sellRequest.Header.Set("Content-Type", "application/json")
	sellRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	sellResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(sellResponse, sellRequest)
	if sellResponse.Code != http.StatusCreated {
		t.Fatalf("sell status = %d, want %d", sellResponse.Code, http.StatusCreated)
	}
	if accounts.appliedFill.Order.Side != paper.OrderSideSell {
		t.Fatalf("applied side = %q, want sell", accounts.appliedFill.Order.Side)
	}
	if len(accounts.snapshot.Positions) != 1 || accounts.snapshot.Positions[0].Quantity != 1 {
		t.Fatalf("positions after sell = %#v, want one 1-share position", accounts.snapshot.Positions)
	}
}

func TestCreateOrderRejectsStaleQuotes(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	accounts := &fakeAccounts{
		snapshot: paper.AccountSnapshot{
			Account: paper.PaperAccount{
				ID:       "paper-account-1",
				UserID:   "google-subject",
				OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
			},
			CashBalanceCents: paper.DefaultStartingCashCents,
		},
	}
	server := NewServer(
		stubQuotes{
			quote: market.Quote{
				Symbol: "AAPL",
				Price:  191.30,
				AsOf:   time.Now().UTC().Add(-8 * 24 * time.Hour),
				Source: "Massive (end-of-day)",
			},
		},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Accounts:      accounts,
		}),
	)

	sessionToken, _, err := sessions.CreateSession(identity.Profile{
		Subject: "google-subject",
		Email:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	orderRequest := httptest.NewRequest(http.MethodPost, ordersPath, strings.NewReader(`{"symbol":"AAPL","quantity":1}`))
	orderRequest.Header.Set("Content-Type", "application/json")
	orderRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	orderResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(orderResponse, orderRequest)
	if orderResponse.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", orderResponse.Code, http.StatusBadGateway)
	}
	var payload map[string]string
	if err := json.NewDecoder(orderResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if payload["error"] != "this quote is too old to execute a paper trade" {
		t.Fatalf("error = %q, want stale quote message", payload["error"])
	}
	if accounts.appliedFill.Order.ID != "" {
		t.Fatalf("stale quote should not persist a fill: %#v", accounts.appliedFill)
	}
}

func TestOrdersRequireAuthenticationAndListHistory(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	realized := paper.Money(9_250)
	accounts := &fakeAccounts{
		orders: []paper.OrderHistoryEntry{
			{
				Order: paper.Order{
					ID:          "order_sell",
					AccountID:   "paper-account-1",
					Symbol:      "AAPL",
					Side:        paper.OrderSideSell,
					Type:        paper.OrderTypeMarket,
					Quantity:    5,
					Status:      paper.OrderStatusFilled,
					SubmittedAt: time.Date(2026, 8, 18, 18, 5, 0, 0, time.UTC),
				},
				Execution: paper.Execution{
					ID:               "execution:order_sell",
					OrderID:          "order_sell",
					Symbol:           "AAPL",
					Quantity:         5,
					PriceCents:       22_900,
					OccurredAt:       time.Date(2026, 8, 18, 18, 5, 0, 0, time.UTC),
					GrossCents:       114_500,
					RealizedPnLCents: &realized,
				},
			},
			{
				Order: paper.Order{
					ID:          "order_buy",
					AccountID:   "paper-account-1",
					Symbol:      "AAPL",
					Side:        paper.OrderSideBuy,
					Type:        paper.OrderTypeMarket,
					Quantity:    10,
					Status:      paper.OrderStatusFilled,
					SubmittedAt: time.Date(2026, 8, 18, 17, 42, 0, 0, time.UTC),
				},
				Execution: paper.Execution{
					ID:         "execution:order_buy",
					OrderID:    "order_buy",
					Symbol:     "AAPL",
					Quantity:   10,
					PriceCents: 22_715,
					OccurredAt: time.Date(2026, 8, 18, 17, 42, 0, 0, time.UTC),
					GrossCents: -227_150,
				},
			},
		},
	}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Accounts:      accounts,
		}),
	)

	unauthenticatedRequest := httptest.NewRequest(http.MethodGet, ordersPath, nil)
	unauthenticatedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthenticatedResponse, unauthenticatedRequest)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticatedResponse.Code, http.StatusUnauthorized)
	}

	sessionToken, _, err := sessions.CreateSession(identity.Profile{
		Subject: "google-subject",
		Email:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	listRequest := httptest.NewRequest(http.MethodGet, ordersPath+"?limit=25", nil)
	listRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	listResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	if accounts.ordersFor != "google-subject" {
		t.Fatalf("orders requested for %q, want google-subject", accounts.ordersFor)
	}
	if accounts.ordersLimit != 25 {
		t.Fatalf("orders limit = %d, want 25", accounts.ordersLimit)
	}
	if accounts.ordersOffset != 0 {
		t.Fatalf("orders offset = %d, want 0", accounts.ordersOffset)
	}

	var payload listOrdersResponse
	if err := json.NewDecoder(listResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if payload.Total != 2 || payload.Limit != 25 || payload.Offset != 0 {
		t.Fatalf("page metadata = total %d limit %d offset %d, want 2/25/0", payload.Total, payload.Limit, payload.Offset)
	}
	if len(payload.Orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(payload.Orders))
	}
	if payload.Orders[0].Side != paper.OrderSideSell {
		t.Fatalf("first side = %q, want sell", payload.Orders[0].Side)
	}
	if payload.Orders[0].Execution.RealizedPnLCents == nil || *payload.Orders[0].Execution.RealizedPnLCents != 9_250 {
		t.Fatalf("sell realized P&L = %v, want 9250", payload.Orders[0].Execution.RealizedPnLCents)
	}
	if payload.Orders[1].Execution.RealizedPnLCents != nil {
		t.Fatalf("buy realized P&L = %v, want nil", payload.Orders[1].Execution.RealizedPnLCents)
	}

	pageRequest := httptest.NewRequest(http.MethodGet, ordersPath+"?limit=1&offset=1", nil)
	pageRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	pageResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(pageResponse, pageRequest)
	if pageResponse.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d", pageResponse.Code, http.StatusOK)
	}
	var pagePayload listOrdersResponse
	if err := json.NewDecoder(pageResponse.Body).Decode(&pagePayload); err != nil {
		t.Fatalf("decode page response: %v", err)
	}
	if pagePayload.Total != 2 || pagePayload.Limit != 1 || pagePayload.Offset != 1 {
		t.Fatalf("second page metadata = total %d limit %d offset %d, want 2/1/1", pagePayload.Total, pagePayload.Limit, pagePayload.Offset)
	}
	if len(pagePayload.Orders) != 1 || pagePayload.Orders[0].ID != "order_buy" {
		t.Fatalf("second page orders = %#v, want order_buy", pagePayload.Orders)
	}

	invalidLimitRequest := httptest.NewRequest(http.MethodGet, ordersPath+"?limit=0", nil)
	invalidLimitRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	invalidLimitResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimitResponse, invalidLimitRequest)
	if invalidLimitResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want %d", invalidLimitResponse.Code, http.StatusBadRequest)
	}

	invalidOffsetRequest := httptest.NewRequest(http.MethodGet, ordersPath+"?offset=-1", nil)
	invalidOffsetRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	invalidOffsetResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidOffsetResponse, invalidOffsetRequest)
	if invalidOffsetResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid offset status = %d, want %d", invalidOffsetResponse.Code, http.StatusBadRequest)
	}
}

func TestAccountActivityRequiresAuthenticationAndListsEntries(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	accounts := &fakeAccounts{
		activity: []paper.LedgerActivityEntry{
			{
				TransactionID: "order-fill:order_1",
				OccurredAt:    time.Date(2026, 8, 10, 14, 30, 0, 0, time.UTC),
				Description:   "Paper purchase of AAPL",
				AmountCents:   -19_130,
			},
			{
				TransactionID: "opening-cash:paper-account-1",
				OccurredAt:    time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
				Description:   "Initial paper cash",
				AmountCents:   paper.DefaultStartingCashCents,
			},
		},
	}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Accounts:      accounts,
		}),
	)

	unauthenticatedRequest := httptest.NewRequest(http.MethodGet, accountActivityPath, nil)
	unauthenticatedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthenticatedResponse, unauthenticatedRequest)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthenticatedResponse.Code, http.StatusUnauthorized)
	}

	sessionToken, _, err := sessions.CreateSession(identity.Profile{
		Subject: "google-subject",
		Email:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	activityRequest := httptest.NewRequest(http.MethodGet, accountActivityPath+"?limit=10", nil)
	activityRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	activityResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(activityResponse, activityRequest)
	if activityResponse.Code != http.StatusOK {
		t.Fatalf("activity status = %d, want %d", activityResponse.Code, http.StatusOK)
	}
	if accounts.activityFor != "google-subject" {
		t.Fatalf("activity requested for %q, want google-subject", accounts.activityFor)
	}
	if accounts.activityLimit != 10 {
		t.Fatalf("activity limit = %d, want 10", accounts.activityLimit)
	}

	invalidLimitRequest := httptest.NewRequest(http.MethodGet, accountActivityPath+"?limit=0", nil)
	invalidLimitRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	invalidLimitResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(invalidLimitResponse, invalidLimitRequest)
	if invalidLimitResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status = %d, want %d", invalidLimitResponse.Code, http.StatusBadRequest)
	}
}

func (f *fakeAccounts) Snapshot(_ context.Context, userID string) (paper.AccountSnapshot, error) {
	f.snapshotFor = append(f.snapshotFor, userID)
	if len(f.snapshotErrs) > 0 {
		err := f.snapshotErrs[0]
		f.snapshotErrs = f.snapshotErrs[1:]
		return f.snapshot, err
	}
	return f.snapshot, f.snapshotErr
}

func (f *fakeAccounts) PostTransaction(_ context.Context, _ paper.Transaction) error {
	return nil
}

func (f *fakeAccounts) ListActivity(_ context.Context, userID string, limit int) ([]paper.LedgerActivityEntry, error) {
	f.activityFor = userID
	f.activityLimit = limit
	return f.activity, f.activityErr
}

func (f *fakeAccounts) ListOrders(_ context.Context, userID string, limit, offset int) (paper.OrderHistoryPage, error) {
	f.ordersFor = userID
	f.ordersLimit = limit
	f.ordersOffset = offset
	if f.ordersErr != nil {
		return paper.OrderHistoryPage{}, f.ordersErr
	}
	orders := f.orders
	if orders == nil {
		orders = []paper.OrderHistoryEntry{}
	}
	total := len(orders)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := []paper.OrderHistoryEntry{}
	if offset < total {
		page = append(page, orders[offset:end]...)
	}
	return paper.OrderHistoryPage{
		Orders: page,
		Total:  total,
		Limit:  limit,
		Offset: f.ordersOffset,
	}, nil
}

func (f *fakeAccounts) ApplyOrderFill(_ context.Context, fill paper.OrderFill) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	f.appliedFill = fill
	f.snapshot.CashBalanceCents += fill.CashTransaction.Postings[0].Amount
	for index, position := range f.snapshot.Positions {
		if position.Symbol != fill.Order.Symbol {
			continue
		}
		if fill.Order.Side == paper.OrderSideBuy {
			f.snapshot.Positions[index].Quantity += fill.Order.Quantity
			f.snapshot.Positions[index].CostBasisCents += -fill.CashTransaction.Postings[0].Amount
		} else {
			f.snapshot.Positions[index].Quantity -= fill.Order.Quantity
			if f.snapshot.Positions[index].Quantity == 0 {
				f.snapshot.Positions = append(f.snapshot.Positions[:index], f.snapshot.Positions[index+1:]...)
			}
		}
		return nil
	}
	if fill.Order.Side == paper.OrderSideBuy {
		f.snapshot.Positions = append(f.snapshot.Positions, paper.Position{
			Symbol:         fill.Order.Symbol,
			Quantity:       fill.Order.Quantity,
			CostBasisCents: -fill.CashTransaction.Postings[0].Amount,
		})
	}
	return nil
}

func (f *fakeGoogleAuthenticator) AuthorizationURL(state, nonce string) string {
	f.state = state
	f.nonce = nonce
	return "https://accounts.google.com/o/oauth2/v2/auth"
}

func (f *fakeGoogleAuthenticator) Authenticate(_ context.Context, code, nonce string) (identity.Profile, error) {
	if code != "authorization-code" {
		return identity.Profile{}, errors.New("unexpected authorization code")
	}
	if nonce != f.nonce {
		return identity.Profile{}, errors.New("unexpected nonce")
	}
	return f.profile, f.err
}

func TestGoogleAuthenticationFlow(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	authenticator := &fakeGoogleAuthenticator{
		profile: identity.Profile{
			Subject: "google-subject",
			Email:   "user@example.com",
			Name:    "Example User",
		},
	}
	accounts := &fakeAccounts{
		account: paper.PaperAccount{
			ID:       "paper-account-1",
			UserID:   "google-subject",
			OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
		},
		snapshot: paper.AccountSnapshot{
			Account: paper.PaperAccount{
				ID:       "paper-account-1",
				UserID:   "google-subject",
				OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
			},
			CashBalanceCents: paper.DefaultStartingCashCents,
		},
	}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator:   authenticator,
			Sessions:        sessions,
			Accounts:        accounts,
			SuccessRedirect: "http://localhost:3000/auth/callback",
		}),
	)

	startRequest := httptest.NewRequest(http.MethodGet, googleAuthStartPath, nil)
	startResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(startResponse, startRequest)
	if startResponse.Code != http.StatusFound {
		t.Fatalf("start status = %d, want %d", startResponse.Code, http.StatusFound)
	}
	stateCookie := cookieNamed(t, startResponse.Result().Cookies(), stateCookieName)

	callbackURL := googleAuthCallbackPath + "?state=" + url.QueryEscape(authenticator.state) + "&code=authorization-code"
	callbackRequest := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	callbackRequest.AddCookie(stateCookie)
	callbackResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want %d", callbackResponse.Code, http.StatusFound)
	}
	if accounts.ensuredFor != "google-subject" {
		t.Fatalf("account was provisioned for %q, want google-subject", accounts.ensuredFor)
	}
	sessionCookie := cookieNamed(t, callbackResponse.Result().Cookies(), sessionCookieName)

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meRequest.AddCookie(sessionCookie)
	meResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d", meResponse.Code, http.StatusOK)
	}

	accountRequest := httptest.NewRequest(http.MethodGet, accountPath, nil)
	accountRequest.AddCookie(sessionCookie)
	accountResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(accountResponse, accountRequest)
	if accountResponse.Code != http.StatusOK {
		t.Fatalf("account status = %d, want %d", accountResponse.Code, http.StatusOK)
	}
}

func TestGoogleAuthenticationRejectsMismatchedState(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator:   &fakeGoogleAuthenticator{},
			Sessions:        sessions,
			SuccessRedirect: "http://localhost:3000",
		}),
	)

	startRequest := httptest.NewRequest(http.MethodGet, googleAuthStartPath, nil)
	startResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(startResponse, startRequest)
	stateCookie := cookieNamed(t, startResponse.Result().Cookies(), stateCookieName)

	callbackRequest := httptest.NewRequest(http.MethodGet, googleAuthCallbackPath+"?state=wrong&code=authorization-code", nil)
	callbackRequest.AddCookie(stateCookie)
	callbackResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(callbackResponse, callbackRequest)
	if callbackResponse.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d", callbackResponse.Code, http.StatusBadRequest)
	}
}

func TestAccountProvisionsMissingAccountForExistingSession(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	accounts := &fakeAccounts{
		account: paper.PaperAccount{
			ID:       "paper-account-1",
			UserID:   "google-subject",
			OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
		},
		snapshot: paper.AccountSnapshot{
			Account: paper.PaperAccount{
				ID:       "paper-account-1",
				UserID:   "google-subject",
				OpenedAt: time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC),
			},
			CashBalanceCents: paper.DefaultStartingCashCents,
		},
		snapshotErrs: []error{paper.ErrAccountNotFound, nil},
	}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Accounts:      accounts,
			SecureCookies: false,
		}),
	)
	sessionToken, _, err := sessions.CreateSession(identity.Profile{
		Subject: "google-subject",
		Email:   "user@example.com",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	accountRequest := httptest.NewRequest(http.MethodGet, accountPath, nil)
	accountRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	accountResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(accountResponse, accountRequest)

	if accountResponse.Code != http.StatusOK {
		t.Fatalf("account status = %d, want %d", accountResponse.Code, http.StatusOK)
	}
	if accounts.ensuredFor != "google-subject" {
		t.Fatalf("account was provisioned for %q, want google-subject", accounts.ensuredFor)
	}
	if len(accounts.snapshotFor) != 2 {
		t.Fatalf("snapshot calls = %d, want 2", len(accounts.snapshotFor))
	}
}

func cookieNamed(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q was not set", name)
	return nil
}
