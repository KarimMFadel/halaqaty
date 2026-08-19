package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoadLiveKitConfig_EmptyWhenUnset(t *testing.T) {
	cfg, err := loadLiveKitConfig(func(string) string { return "" })

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if cfg != (LiveKitConfig{}) {
		t.Fatalf("config: got %+v, want zero value", cfg)
	}
}

func TestLoadLiveKitConfig_Success(t *testing.T) {
	env := map[string]string{
		"LIVEKIT_ENDPOINT":   "wss://livekit.example.com",
		"LIVEKIT_API_KEY":    "test-api-key",
		"LIVEKIT_API_SECRET": "test-api-secret",
	}
	cfg, err := loadLiveKitConfig(func(key string) string { return env[key] })

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if cfg.Endpoint != "wss://livekit.example.com" {
		t.Fatalf("endpoint: got %q", cfg.Endpoint)
	}
	if cfg.APIKey != "test-api-key" {
		t.Fatalf("api key: got %q", cfg.APIKey)
	}
	if cfg.APISecret != "test-api-secret" {
		t.Fatalf("api secret: got %q", cfg.APISecret)
	}
}

func TestLoadLiveKitConfig_ReadsEnvironment(t *testing.T) {
	t.Setenv("LIVEKIT_ENDPOINT", "https://livekit.example.com")
	t.Setenv("LIVEKIT_API_KEY", "test-api-key")
	t.Setenv("LIVEKIT_API_SECRET", "test-api-secret")

	cfg, err := LoadLiveKitConfig()

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if cfg.Endpoint != "https://livekit.example.com" {
		t.Fatalf("endpoint: got %q, want env value", cfg.Endpoint)
	}
}

func TestLoadLiveKitConfig_MissingValuesWhenRequired(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "key and secret set without endpoint",
			env: map[string]string{
				"LIVEKIT_API_KEY":    "test-api-key",
				"LIVEKIT_API_SECRET": "test-api-secret",
			},
			want: "LIVEKIT_ENDPOINT is required",
		},
		{
			name: "endpoint and secret set without key",
			env: map[string]string{
				"LIVEKIT_ENDPOINT":   "https://livekit.example.com",
				"LIVEKIT_API_SECRET": "test-api-secret",
			},
			want: "LIVEKIT_API_KEY is required",
		},
		{
			name: "endpoint and key set without secret",
			env: map[string]string{
				"LIVEKIT_ENDPOINT": "https://livekit.example.com",
				"LIVEKIT_API_KEY":  "test-api-key",
			},
			want: "LIVEKIT_API_SECRET is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadLiveKitConfig(func(key string) string { return tt.env[key] })

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestLoadLiveKitConfig_RejectsNonTLSAndInvalidEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "http scheme", endpoint: "http://livekit.example.com", want: "https or wss scheme"},
		{name: "ws scheme", endpoint: "ws://livekit.example.com", want: "https or wss scheme"},
		{name: "missing scheme", endpoint: "livekit.example.com", want: "https or wss scheme"},
		{name: "missing host", endpoint: "wss:///path", want: "include a host"},
		{name: "unparseable", endpoint: "wss://bad\x7furl", want: "invalid URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{
				"LIVEKIT_ENDPOINT":   tt.endpoint,
				"LIVEKIT_API_KEY":    "test-api-key",
				"LIVEKIT_API_SECRET": "test-api-secret",
			}
			_, err := loadLiveKitConfig(func(key string) string { return env[key] })

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestLiveKitConfig_SecretNeverFormatted guards the constitution §IV secret
// invariant: no formatting of the config may expose APISecret, so a future
// String()/GoString()/MarshalJSON change that includes it fails here. %#v
// (raw Go-syntax dump) is covered because GoString returns the redacted
// String representation.
func TestLiveKitConfig_SecretNeverFormatted(t *testing.T) {
	cfg := LiveKitConfig{
		Endpoint:  "wss://livekit.example.com",
		APIKey:    "test-api-key",
		APISecret: "super-secret-do-not-print",
	}

	for _, rendered := range []string{
		fmt.Sprint(cfg),
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		cfg.String(),
		cfg.GoString(),
	} {
		if strings.Contains(rendered, cfg.APISecret) {
			t.Fatalf("formatted config leaks API secret: %q", rendered)
		}
	}
}

// TestLiveKitConfig_ErrorsNeverContainSecret guards that validation error
// messages never echo the secret value.
func TestLiveKitConfig_ErrorsNeverContainSecret(t *testing.T) {
	env := map[string]string{
		"LIVEKIT_ENDPOINT":   "http://insecure.example.com",
		"LIVEKIT_API_KEY":    "test-api-key",
		"LIVEKIT_API_SECRET": "super-secret-do-not-print",
	}
	_, err := loadLiveKitConfig(func(key string) string { return env[key] })

	if err == nil {
		t.Fatal("error: got nil, want validation failure")
	}
	if strings.Contains(err.Error(), env["LIVEKIT_API_SECRET"]) {
		t.Fatalf("error message leaks API secret: %q", err.Error())
	}
}

func TestLoadAudioPolicy_Defaults(t *testing.T) {
	policy, err := loadAudioPolicy(func(string) string { return "" })

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	want := AudioPolicy{OpusBitrateKbps: 48, NoiseSuppression: false, AutoGainControl: false, EchoCancellation: false}
	if policy != want {
		t.Fatalf("policy: got %+v, want %+v", policy, want)
	}
}

func TestLoadAudioPolicy_Overrides(t *testing.T) {
	env := map[string]string{
		"LIVEKIT_AUDIO_OPUS_BITRATE_KBPS": "64",
	}
	policy, err := loadAudioPolicy(func(key string) string { return env[key] })

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if policy.OpusBitrateKbps != 64 {
		t.Fatalf("bitrate: got %d, want 64", policy.OpusBitrateKbps)
	}
}

func TestLoadAudioPolicy_ReadsEnvironment(t *testing.T) {
	t.Setenv("LIVEKIT_AUDIO_OPUS_BITRATE_KBPS", "96")

	policy, err := LoadAudioPolicy()

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if policy.OpusBitrateKbps != 96 {
		t.Fatalf("bitrate: got %d, want env value 96", policy.OpusBitrateKbps)
	}
}

func TestLoadAudioPolicy_RejectsLowBitrate(t *testing.T) {
	env := map[string]string{
		"LIVEKIT_AUDIO_OPUS_BITRATE_KBPS": "47",
	}
	_, err := loadAudioPolicy(func(key string) string { return env[key] })

	if err == nil {
		t.Fatal("error: got nil, want validation failure")
	}
	if !strings.Contains(err.Error(), ">= 48") {
		t.Fatalf("error: got %q, want it to contain the minimum bitrate", err.Error())
	}
}

func TestLoadAudioPolicy_RejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "non-integer bitrate",
			env:  map[string]string{"LIVEKIT_AUDIO_OPUS_BITRATE_KBPS": "loud"},
			want: "LIVEKIT_AUDIO_OPUS_BITRATE_KBPS invalid integer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadAudioPolicy(func(key string) string { return tt.env[key] })

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// TestLoadAudioPolicy_ConstitutionFixed guards constitution §V: noise
// suppression, automatic gain control, and echo cancellation are not operator
// knobs. DefaultAudioPolicy is their only source, so stale environment
// entries named after the removed overrides must have no effect.
func TestLoadAudioPolicy_ConstitutionFixed(t *testing.T) {
	t.Setenv("LIVEKIT_AUDIO_NOISE_SUPPRESSION", "true")
	t.Setenv("LIVEKIT_AUDIO_AUTO_GAIN_CONTROL", "true")
	t.Setenv("LIVEKIT_AUDIO_ECHO_CANCELLATION", "true")

	policy, err := LoadAudioPolicy()

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if policy.NoiseSuppression || policy.AutoGainControl || policy.EchoCancellation {
		t.Fatalf("audio processing must stay disabled regardless of environment: %+v", policy)
	}
}
