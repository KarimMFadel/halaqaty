package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// AuthConfig centralizes auth/session/rate-limit/timeout settings.
type AuthConfig struct {
	FirebaseProjectID      string
	SessionInactivityTTL   time.Duration
	SessionAbsoluteTTL     time.Duration
	RequestTimeout         time.Duration
	RateLimitPerIPPerMin   int
	RateLimitPerUserPerMin int
	WSMaxConnectionsPerUsr int
	WSMaxMessagesPerMin    int
	MetricsToken           string
}

// DefaultAuthConfig provides production-safe defaults.
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		SessionInactivityTTL:   30 * 24 * time.Hour,
		SessionAbsoluteTTL:     90 * 24 * time.Hour,
		RequestTimeout:         15 * time.Second,
		RateLimitPerIPPerMin:   120,
		RateLimitPerUserPerMin: 240,
		WSMaxConnectionsPerUsr: 3,
		WSMaxMessagesPerMin:    30,
	}
}

// LoadAuthConfig reads env overrides for auth and rate-limit behavior.
func LoadAuthConfig() (AuthConfig, error) {
	return loadAuthConfig(os.Getenv)
}

func loadAuthConfig(getenv func(string) string) (AuthConfig, error) {
	cfg := DefaultAuthConfig()
	cfg.FirebaseProjectID = getenv("FIREBASE_PROJECT_ID")
	cfg.MetricsToken = getenv("METRICS_TOKEN")

	var err error
	cfg.SessionInactivityTTL, err = envDuration(getenv, "AUTH_SESSION_INACTIVITY_TTL", cfg.SessionInactivityTTL)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.SessionAbsoluteTTL, err = envDuration(getenv, "AUTH_SESSION_ABSOLUTE_TTL", cfg.SessionAbsoluteTTL)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.RequestTimeout, err = envDuration(getenv, "AUTH_REQUEST_TIMEOUT", cfg.RequestTimeout)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.RateLimitPerIPPerMin, err = envInt(getenv, "AUTH_RATE_LIMIT_IP_PER_MIN", cfg.RateLimitPerIPPerMin)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.RateLimitPerUserPerMin, err = envInt(getenv, "AUTH_RATE_LIMIT_USER_PER_MIN", cfg.RateLimitPerUserPerMin)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.WSMaxConnectionsPerUsr, err = envInt(getenv, "AUTH_WS_MAX_CONNECTIONS_PER_USER", cfg.WSMaxConnectionsPerUsr)
	if err != nil {
		return AuthConfig{}, err
	}
	cfg.WSMaxMessagesPerMin, err = envInt(getenv, "AUTH_WS_MAX_MESSAGES_PER_MIN", cfg.WSMaxMessagesPerMin)
	if err != nil {
		return AuthConfig{}, err
	}

	return cfg, nil
}

func envDuration(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}

	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s invalid duration %q: %w", key, raw, err)
	}
	if dur <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}

	return dur, nil
}

func envInt(getenv func(string) string, key string, fallback int) (int, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s invalid integer %q: %w", key, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be > 0", key)
	}

	return value, nil
}
