package identity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	oauthStateLifetime = 10 * time.Minute
	sessionLifetime    = 24 * time.Hour
)

var (
	ErrInvalidToken = errors.New("invalid session token")
	ErrExpiredToken = errors.New("expired session token")
)

type SessionManager struct {
	secret []byte
	now    func() time.Time
}

type Profile struct {
	Subject string `json:"subject"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture,omitempty"`
}

type Session struct {
	Profile
	ExpiresAt time.Time `json:"expires_at"`
}

type oauthState struct {
	Value     string    `json:"value"`
	Nonce     string    `json:"nonce"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewSessionManager(secret string) (*SessionManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("AUTH_SESSION_SECRET must be at least 32 bytes")
	}

	return &SessionManager{
		secret: []byte(secret),
		now:    time.Now,
	}, nil
}

func (m *SessionManager) NewOAuthState() (state, nonce, token string, err error) {
	state, err = randomValue()
	if err != nil {
		return "", "", "", err
	}
	nonce, err = randomValue()
	if err != nil {
		return "", "", "", err
	}

	token, err = m.sign(oauthState{
		Value:     state,
		Nonce:     nonce,
		ExpiresAt: m.now().Add(oauthStateLifetime),
	})
	if err != nil {
		return "", "", "", err
	}
	return state, nonce, token, nil
}

func (m *SessionManager) VerifyOAuthState(token, state string) (string, error) {
	var stored oauthState
	if err := m.verify(token, &stored); err != nil {
		return "", err
	}
	if m.now().After(stored.ExpiresAt) {
		return "", ErrExpiredToken
	}
	if !hmac.Equal([]byte(stored.Value), []byte(state)) {
		return "", ErrInvalidToken
	}
	return stored.Nonce, nil
}

func (m *SessionManager) CreateSession(profile Profile) (string, Session, error) {
	session := Session{
		Profile:   profile,
		ExpiresAt: m.now().Add(sessionLifetime),
	}
	token, err := m.sign(session)
	if err != nil {
		return "", Session{}, err
	}
	return token, session, nil
}

func (m *SessionManager) VerifySession(token string) (Session, error) {
	var session Session
	if err := m.verify(token, &session); err != nil {
		return Session{}, err
	}
	if m.now().After(session.ExpiresAt) {
		return Session{}, ErrExpiredToken
	}
	if session.Subject == "" || session.Email == "" {
		return Session{}, ErrInvalidToken
	}
	return session, nil
}

func (m *SessionManager) sign(payload any) (string, error) {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode signed token: %w", err)
	}

	payloadPart := base64.RawURLEncoding.EncodeToString(encodedPayload)
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payloadPart))
	signaturePart := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payloadPart + "." + signaturePart, nil
}

func (m *SessionManager) verify(token string, destination any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ErrInvalidToken
	}

	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(parts[0]))
	expectedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expectedSignature, mac.Sum(nil)) {
		return ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrInvalidToken
	}
	if err := json.Unmarshal(payload, destination); err != nil {
		return ErrInvalidToken
	}
	return nil
}

func randomValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
