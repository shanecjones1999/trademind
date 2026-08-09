package identity

import (
	"errors"
	"testing"
	"time"
)

func TestSessionManagerCreatesAndVerifiesSession(t *testing.T) {
	manager, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	token, _, err := manager.CreateSession(Profile{
		Subject: "google-user-id",
		Email:   "user@example.com",
		Name:    "Example User",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	session, err := manager.VerifySession(token)
	if err != nil {
		t.Fatalf("verify session: %v", err)
	}
	if session.Subject != "google-user-id" || session.Email != "user@example.com" {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestSessionManagerRejectsTamperedToken(t *testing.T) {
	manager, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	token, _, err := manager.CreateSession(Profile{Subject: "user", Email: "user@example.com"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err = manager.VerifySession(token + "x")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("verify session error = %v, want ErrInvalidToken", err)
	}
}

func TestSessionManagerRejectsExpiredSession(t *testing.T) {
	manager, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	now := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }

	token, _, err := manager.CreateSession(Profile{Subject: "user", Email: "user@example.com"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	manager.now = func() time.Time { return now.Add(sessionLifetime + time.Second) }

	_, err = manager.VerifySession(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("verify session error = %v, want ErrExpiredToken", err)
	}
}
