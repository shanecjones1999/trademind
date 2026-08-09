package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr           string
	AllowedOrigins     []string
	MassiveAPIKey      string
	DatabaseURL        string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	AppWebURL          string
	AuthSessionSecret  string
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	config := Config{
		HTTPAddr:           valueOrDefault("HTTP_ADDR", ":8080"),
		AllowedOrigins:     commaSeparatedValues(valueOrDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		MassiveAPIKey:      strings.TrimSpace(os.Getenv("MASSIVE_API_KEY")),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		GoogleClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		GoogleRedirectURL:  valueOrDefault("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		AppWebURL:          valueOrDefault("APP_WEB_URL", "http://localhost:3000/dashboard"),
		AuthSessionSecret:  strings.TrimSpace(os.Getenv("AUTH_SESSION_SECRET")),
	}
	if config.GoogleAuthEnabled() {
		if !isAbsoluteURL(config.GoogleRedirectURL) {
			return Config{}, fmt.Errorf("GOOGLE_REDIRECT_URL must be an absolute URL")
		}
		if !isAbsoluteURL(config.AppWebURL) {
			return Config{}, fmt.Errorf("APP_WEB_URL must be an absolute URL")
		}
	}

	return config, nil
}

func (c Config) GoogleAuthEnabled() bool {
	return c.GoogleClientID != "" &&
		c.GoogleClientSecret != "" &&
		c.GoogleRedirectURL != "" &&
		c.AppWebURL != "" &&
		c.AuthSessionSecret != ""
}

func valueOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func commaSeparatedValues(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func isAbsoluteURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}
