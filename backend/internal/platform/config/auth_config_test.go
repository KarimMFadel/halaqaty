package config

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestLoadSessionRoomKey_ReturnsDecodedKey(t *testing.T) {
	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i)
	}
	getenv := func(string) string { return base64.StdEncoding.EncodeToString(want) }

	key, err := loadSessionRoomKey(getenv)
	if err != nil {
		t.Fatalf("loadSessionRoomKey: %v", err)
	}
	if string(key) != string(want) {
		t.Fatalf("decoded key mismatch")
	}
}

func TestLoadSessionRoomKey_RequiresKey(t *testing.T) {
	_, err := loadSessionRoomKey(func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error when key is missing")
	}
}

func TestLoadSessionRoomKey_RequiresBase64(t *testing.T) {
	_, err := loadSessionRoomKey(func(string) string { return "not-base64!!!" })
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestLoadSessionRoomKey_RequiresMinimumLength(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	_, err := loadSessionRoomKey(func(string) string { return short })
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestDefaultAuthConfig_ProvidesProductionDefaults(t *testing.T) {
	cfg := DefaultAuthConfig()
	if cfg.SessionInactivityTTL != 30*24*time.Hour {
		t.Fatalf("SessionInactivityTTL: got %v", cfg.SessionInactivityTTL)
	}
	if cfg.SessionAbsoluteTTL != 90*24*time.Hour {
		t.Fatalf("SessionAbsoluteTTL: got %v", cfg.SessionAbsoluteTTL)
	}
	if cfg.RateLimitPerIPPerMin != 120 || cfg.RateLimitPerUserPerMin != 240 {
		t.Fatalf("rate limits: got %+v", cfg)
	}
}

func TestLoadAuthConfig_AppliesEnvOverrides(t *testing.T) {
	env := map[string]string{
		"FIREBASE_PROJECT_ID":              "halaqaty-test",
		"METRICS_TOKEN":                    "secret-token",
		"AUTH_SESSION_INACTIVITY_TTL":      "1h",
		"AUTH_SESSION_ABSOLUTE_TTL":        "2h",
		"AUTH_REQUEST_TIMEOUT":             "3s",
		"AUTH_RATE_LIMIT_IP_PER_MIN":       "10",
		"AUTH_RATE_LIMIT_USER_PER_MIN":     "20",
		"AUTH_WS_MAX_CONNECTIONS_PER_USER": "5",
		"AUTH_WS_MAX_MESSAGES_PER_MIN":     "50",
	}
	getenv := func(key string) string { return env[key] }

	cfg, err := loadAuthConfig(getenv)
	if err != nil {
		t.Fatalf("loadAuthConfig: %v", err)
	}
	if cfg.FirebaseProjectID != "halaqaty-test" {
		t.Fatalf("FirebaseProjectID: got %q", cfg.FirebaseProjectID)
	}
	if cfg.MetricsToken != "secret-token" {
		t.Fatalf("MetricsToken: got %q", cfg.MetricsToken)
	}
	if cfg.SessionInactivityTTL != time.Hour {
		t.Fatalf("SessionInactivityTTL: got %v", cfg.SessionInactivityTTL)
	}
	if cfg.SessionAbsoluteTTL != 2*time.Hour {
		t.Fatalf("SessionAbsoluteTTL: got %v", cfg.SessionAbsoluteTTL)
	}
	if cfg.RequestTimeout != 3*time.Second {
		t.Fatalf("RequestTimeout: got %v", cfg.RequestTimeout)
	}
	if cfg.RateLimitPerIPPerMin != 10 || cfg.RateLimitPerUserPerMin != 20 {
		t.Fatalf("rate limits: got %+v", cfg)
	}
	if cfg.WSMaxConnectionsPerUsr != 5 || cfg.WSMaxMessagesPerMin != 50 {
		t.Fatalf("ws limits: got %+v", cfg)
	}
}

func TestLoadAuthConfig_UsesDefaultsForMissingEnv(t *testing.T) {
	cfg, err := loadAuthConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("loadAuthConfig: %v", err)
	}
	defaultCfg := DefaultAuthConfig()
	if cfg != defaultCfg {
		t.Fatalf("config mismatch: got %+v want %+v", cfg, defaultCfg)
	}
}

func TestLoadAuthConfig_RejectsInvalidDurations(t *testing.T) {
	cases := []struct {
		key string
	}{
		{"AUTH_SESSION_INACTIVITY_TTL"},
		{"AUTH_SESSION_ABSOLUTE_TTL"},
		{"AUTH_REQUEST_TIMEOUT"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			getenv := func(key string) string {
				if key == tc.key {
					return "not-a-duration"
				}
				return ""
			}
			_, err := loadAuthConfig(getenv)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadAuthConfig_RejectsNonPositiveDurations(t *testing.T) {
	getenv := func(key string) string {
		if key == "AUTH_SESSION_INACTIVITY_TTL" {
			return "0s"
		}
		return ""
	}
	_, err := loadAuthConfig(getenv)
	if err == nil {
		t.Fatal("expected error for zero duration")
	}
}

func TestLoadAuthConfig_RejectsInvalidInts(t *testing.T) {
	getenv := func(key string) string {
		if key == "AUTH_RATE_LIMIT_IP_PER_MIN" {
			return "abc"
		}
		return ""
	}
	_, err := loadAuthConfig(getenv)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadAuthConfig_RejectsNonPositiveInts(t *testing.T) {
	getenv := func(key string) string {
		if key == "AUTH_RATE_LIMIT_USER_PER_MIN" {
			return "-1"
		}
		return ""
	}
	_, err := loadAuthConfig(getenv)
	if err == nil {
		t.Fatal("expected error for negative integer")
	}
}

func TestEnvDuration_FallbackAndValidation(t *testing.T) {
	cases := []struct {
		name     string
		getenv   func(string) string
		fallback time.Duration
		want     time.Duration
		wantErr  bool
	}{
		{
			name:     "empty uses fallback",
			getenv:   func(string) string { return "" },
			fallback: time.Minute,
			want:     time.Minute,
		},
		{
			name:     "valid override",
			getenv:   func(string) string { return "5m" },
			fallback: time.Minute,
			want:     5 * time.Minute,
		},
		{
			name:     "invalid format",
			getenv:   func(string) string { return "bad" },
			fallback: time.Minute,
			wantErr:  true,
		},
		{
			name:     "non-positive",
			getenv:   func(string) string { return "-1s" },
			fallback: time.Minute,
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := envDuration(tc.getenv, "TEST_DUR", tc.fallback)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err got %v wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("duration: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEnvInt_FallbackAndValidation(t *testing.T) {
	cases := []struct {
		name     string
		getenv   func(string) string
		fallback int
		want     int
		wantErr  bool
	}{
		{
			name:     "empty uses fallback",
			getenv:   func(string) string { return "" },
			fallback: 10,
			want:     10,
		},
		{
			name:     "valid override",
			getenv:   func(string) string { return "42" },
			fallback: 10,
			want:     42,
		},
		{
			name:     "invalid format",
			getenv:   func(string) string { return "xyz" },
			fallback: 10,
			wantErr:  true,
		},
		{
			name:     "non-positive",
			getenv:   func(string) string { return "0" },
			fallback: 10,
			wantErr:  true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := envInt(tc.getenv, "TEST_INT", tc.fallback)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err got %v wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("int: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestLoadAuthConfig_PublicWrappersUseOSEnv(t *testing.T) {
	t.Setenv("AUTH_REQUEST_TIMEOUT", "7s")
	cfg, err := LoadAuthConfig()
	if err != nil {
		t.Fatalf("LoadAuthConfig: %v", err)
	}
	if cfg.RequestTimeout != 7*time.Second {
		t.Fatalf("RequestTimeout: got %v", cfg.RequestTimeout)
	}
}

func TestLoadSessionRoomKey_PublicWrapperUsesOSEnv(t *testing.T) {
	key := make([]byte, 32)
	t.Setenv("SESSION_MEDIA_ROOM_HMAC_KEY", base64.StdEncoding.EncodeToString(key))
	got, err := LoadSessionRoomKey()
	if err != nil {
		t.Fatalf("LoadSessionRoomKey: %v", err)
	}
	if !errors.Is(nil, nil) || string(got) != string(key) {
		t.Fatalf("key mismatch")
	}
}
