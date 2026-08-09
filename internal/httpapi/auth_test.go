package httpapi

import (
	"context"
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
	account      paper.PaperAccount
	snapshot     paper.AccountSnapshot
	ensureErr    error
	snapshotErr  error
	snapshotErrs []error
	applyErr     error
	appliedFill  paper.OrderFill
	ensuredFor   string
	snapshotFor  []string
}

type fakeWatchlists struct {
	watchlists []paper.Watchlist
}

func (f *fakeWatchlists) ListWatchlists(_ context.Context, _ string) ([]paper.Watchlist, error) {
	return f.watchlists, nil
}

func (f *fakeWatchlists) CreateWatchlist(_ context.Context, userID, name string) (paper.Watchlist, error) {
	watchlist := paper.Watchlist{ID: "watchlist-1", UserID: userID, Name: name, Symbols: []paper.WatchlistSymbol{}}
	f.watchlists = append(f.watchlists, watchlist)
	return watchlist, nil
}

func (f *fakeWatchlists) AddWatchlistSymbol(_ context.Context, _ string, _ string, symbol string) (paper.WatchlistSymbol, error) {
	return paper.WatchlistSymbol{Symbol: symbol}, nil
}

func (f *fakeWatchlists) RemoveWatchlistSymbol(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func (f *fakeAccounts) EnsureAccount(_ context.Context, userID string) (paper.PaperAccount, error) {
	f.ensuredFor = userID
	return f.account, f.ensureErr
}

func TestWatchlistsRequireAuthenticationAndSupportCreation(t *testing.T) {
	sessions, err := identity.NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	watchlists := &fakeWatchlists{}
	server := NewServer(
		stubQuotes{},
		nil,
		slog.Default(),
		WithGoogleAuth(GoogleAuthConfig{
			Authenticator: &fakeGoogleAuthenticator{},
			Sessions:      sessions,
			Watchlists:    watchlists,
		}),
	)

	unauthenticatedRequest := httptest.NewRequest(http.MethodGet, watchlistsPath, nil)
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
	createRequest := httptest.NewRequest(http.MethodPost, watchlistsPath, strings.NewReader(`{"name":"Technology"}`))
	createRequest.Header.Set("Content-Type", "application/json")
	createRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	createResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createResponse.Code, http.StatusCreated)
	}

	symbolRequest := httptest.NewRequest(http.MethodPost, watchlistsPath+"/watchlist-1/symbols", strings.NewReader(`{"symbol":"aapl"}`))
	symbolRequest.Header.Set("Content-Type", "application/json")
	symbolRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	symbolResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(symbolResponse, symbolRequest)
	if symbolResponse.Code != http.StatusCreated {
		t.Fatalf("symbol status = %d, want %d", symbolResponse.Code, http.StatusCreated)
	}
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
