package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

var ErrAuthenticationFailed = errors.New("Google authentication failed")

type GoogleAuthenticator interface {
	AuthorizationURL(state, nonce string) string
	Authenticate(ctx context.Context, code, nonce string) (Profile, error)
}

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type GoogleOIDCAuthenticator struct {
	oauthConfig oauth2.Config
	verifier    *oidc.IDTokenVerifier
}

func NewGoogleOIDCAuthenticator(ctx context.Context, config GoogleConfig) (*GoogleOIDCAuthenticator, error) {
	if config.ClientID == "" || config.ClientSecret == "" || config.RedirectURL == "" {
		return nil, errors.New("Google OAuth configuration is incomplete")
	}

	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		return nil, fmt.Errorf("discover Google OpenID Connect provider: %w", err)
	}

	return &GoogleOIDCAuthenticator{
		oauthConfig: oauth2.Config{
			ClientID:     config.ClientID,
			ClientSecret: config.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  config.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
	}, nil
}

func (a *GoogleOIDCAuthenticator) AuthorizationURL(state, nonce string) string {
	return a.oauthConfig.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("nonce", nonce),
	)
}

func (a *GoogleOIDCAuthenticator) Authenticate(ctx context.Context, code, nonce string) (Profile, error) {
	token, err := a.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: exchange authorization code: %v", ErrAuthenticationFailed, err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Profile{}, fmt.Errorf("%w: ID token is missing", ErrAuthenticationFailed)
	}

	idToken, err := a.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: verify ID token: %v", ErrAuthenticationFailed, err)
	}
	if idToken.Nonce != nonce {
		return Profile{}, fmt.Errorf("%w: nonce does not match", ErrAuthenticationFailed)
	}

	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Profile{}, fmt.Errorf("%w: decode claims: %v", ErrAuthenticationFailed, err)
	}
	if claims.Subject == "" || claims.Email == "" || !claims.EmailVerified {
		return Profile{}, fmt.Errorf("%w: verified email is required", ErrAuthenticationFailed)
	}

	return Profile{
		Subject: claims.Subject,
		Email:   claims.Email,
		Name:    claims.Name,
		Picture: claims.Picture,
	}, nil
}
