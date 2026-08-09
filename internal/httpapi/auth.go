package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/shanecjones1999/trademind/internal/identity"
	"github.com/shanecjones1999/trademind/internal/paper"
)

func (s *Server) googleAuthStart(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}

	state, nonce, stateToken, err := s.googleAuth.Sessions.NewOAuthState()
	if err != nil {
		s.logger.Error("create Google OAuth state", "error", err)
		writeError(writer, http.StatusInternalServerError, "unable to start authentication")
		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     stateCookieName,
		Value:    stateToken,
		Path:     googleAuthStartPath,
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   s.googleAuth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(writer, request, s.googleAuth.Authenticator.AuthorizationURL(state, nonce), http.StatusFound)
}

func (s *Server) googleAuthCallback(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}

	stateCookie, err := request.Cookie(stateCookieName)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "authentication state is missing")
		return
	}
	deleteStateCookie(writer, s.googleAuth.SecureCookies)

	nonce, err := s.googleAuth.Sessions.VerifyOAuthState(stateCookie.Value, request.URL.Query().Get("state"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "authentication state is invalid or expired")
		return
	}

	if providerError := request.URL.Query().Get("error"); providerError != "" {
		writeError(writer, http.StatusUnauthorized, "Google authentication was not completed")
		return
	}
	code := request.URL.Query().Get("code")
	if code == "" {
		writeError(writer, http.StatusBadRequest, "authorization code is missing")
		return
	}

	profile, err := s.googleAuth.Authenticator.Authenticate(request.Context(), code, nonce)
	if err != nil {
		s.logger.Error("complete Google authentication", "error", err)
		writeError(writer, http.StatusUnauthorized, "Google authentication failed")
		return
	}
	if s.googleAuth.Accounts != nil {
		if _, err := s.googleAuth.Accounts.EnsureAccount(request.Context(), profile.Subject); err != nil {
			s.logger.Error("provision paper account", "error", err)
			writeError(writer, http.StatusServiceUnavailable, "unable to prepare paper account")
			return
		}
	}

	sessionToken, session, err := s.googleAuth.Sessions.CreateSession(profile)
	if err != nil {
		s.logger.Error("create authenticated session", "error", err)
		writeError(writer, http.StatusInternalServerError, "unable to create session")
		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   int(time.Until(session.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   s.googleAuth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(writer, request, s.googleAuth.SuccessRedirect, http.StatusFound)
}

func (s *Server) me(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}

	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, session.Profile)
}

func (s *Server) account(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}
	if s.googleAuth.Accounts == nil {
		writeError(writer, http.StatusServiceUnavailable, "paper accounts are not configured")
		return
	}

	session, ok := s.sessionFromRequest(writer, request)
	if !ok {
		return
	}
	snapshot, err := s.loadAccountSnapshot(request.Context(), session.Subject)
	if err != nil {
		s.logger.Error("load paper account", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "unable to load paper account")
		return
	}
	writeJSON(writer, http.StatusOK, snapshot)
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	if !s.authenticationConfigured(writer) {
		return
	}

	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.googleAuth.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) authenticationConfigured(writer http.ResponseWriter) bool {
	if s.googleAuth == nil || s.googleAuth.Authenticator == nil || s.googleAuth.Sessions == nil {
		writeError(writer, http.StatusServiceUnavailable, "authentication is not configured")
		return false
	}
	return true
}

func (s *Server) sessionFromRequest(writer http.ResponseWriter, request *http.Request) (identity.Session, bool) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "authentication is required")
		return identity.Session{}, false
	}
	session, err := s.googleAuth.Sessions.VerifySession(cookie.Value)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, identity.ErrInvalidToken) && !errors.Is(err, identity.ErrExpiredToken) {
			status = http.StatusInternalServerError
			s.logger.Error("verify session", "error", err)
		}
		writeError(writer, status, "authentication is required")
		return identity.Session{}, false
	}
	return session, true
}

func deleteStateCookie(writer http.ResponseWriter, secure bool) {
	http.SetCookie(writer, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     googleAuthStartPath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) loadAccountSnapshot(ctx context.Context, userID string) (paper.AccountSnapshot, error) {
	snapshot, err := s.googleAuth.Accounts.Snapshot(ctx, userID)
	if errors.Is(err, paper.ErrAccountNotFound) {
		if _, ensureErr := s.googleAuth.Accounts.EnsureAccount(ctx, userID); ensureErr != nil {
			return paper.AccountSnapshot{}, ensureErr
		}
		snapshot, err = s.googleAuth.Accounts.Snapshot(ctx, userID)
	}
	return snapshot, err
}
