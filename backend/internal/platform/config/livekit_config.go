package config

import (
	"fmt"
	"net/url"
	"os"
)

// MinLiveKitOpusBitrateKbps is the lowest permitted Opus bitrate for Quran
// recitation audio; the constitution (§V) requires 48 kbps or higher.
const MinLiveKitOpusBitrateKbps = 48

// LiveKitConfig holds the validated LiveKit provider connection settings.
// APISecret is a credential: it must never be logged or formatted. String
// and GoString redact it so default formatting (fmt %v/%+v/%#v, slog) stays
// safe.
type LiveKitConfig struct {
	Endpoint  string
	APIKey    string
	APISecret string
}

// String returns a log-safe representation with the API secret redacted.
func (c LiveKitConfig) String() string {
	return fmt.Sprintf("LiveKitConfig{Endpoint:%q APIKey:%q APISecret:[REDACTED]}", c.Endpoint, c.APIKey)
}

// GoString returns the same redacted representation so %#v formatting cannot
// leak the API secret.
func (c LiveKitConfig) GoString() string { return c.String() }

// AudioPolicy holds the Quran audio fidelity settings required by the
// constitution (§V) and FR-008: Opus at 48 kbps or higher with noise
// suppression, automatic gain control, and echo cancellation disabled.
type AudioPolicy struct {
	OpusBitrateKbps  int
	NoiseSuppression bool
	AutoGainControl  bool
	EchoCancellation bool
}

// DefaultAudioPolicy returns the production-safe audio fidelity defaults.
func DefaultAudioPolicy() AudioPolicy {
	return AudioPolicy{
		OpusBitrateKbps:  MinLiveKitOpusBitrateKbps,
		NoiseSuppression: false,
		AutoGainControl:  false,
		EchoCancellation: false,
	}
}

// LoadLiveKitConfig reads and validates the LiveKit endpoint and credentials
// from the environment. The three values are optional as a set: when none is
// set the zero config is returned (live sessions disabled). When any value is
// set, all three are required, and the endpoint must be a trusted TLS URL
// (https or wss scheme).
func LoadLiveKitConfig() (LiveKitConfig, error) {
	return loadLiveKitConfig(os.Getenv)
}

func loadLiveKitConfig(getenv func(string) string) (LiveKitConfig, error) {
	cfg := LiveKitConfig{
		Endpoint:  getenv("LIVEKIT_ENDPOINT"),
		APIKey:    getenv("LIVEKIT_API_KEY"),
		APISecret: getenv("LIVEKIT_API_SECRET"),
	}
	if cfg == (LiveKitConfig{}) {
		return LiveKitConfig{}, nil
	}
	if cfg.Endpoint == "" {
		return LiveKitConfig{}, fmt.Errorf("LIVEKIT_ENDPOINT is required when LiveKit credentials are configured")
	}
	if cfg.APIKey == "" {
		return LiveKitConfig{}, fmt.Errorf("LIVEKIT_API_KEY is required when LiveKit credentials are configured")
	}
	if cfg.APISecret == "" {
		return LiveKitConfig{}, fmt.Errorf("LIVEKIT_API_SECRET is required when LiveKit credentials are configured")
	}
	if err := validateLiveKitEndpoint(cfg.Endpoint); err != nil {
		return LiveKitConfig{}, err
	}
	return cfg, nil
}

// validateLiveKitEndpoint enforces a parseable TLS-only (https/wss) endpoint
// with a host, per the trusted-TLS-endpoint assumption in the F-005 spec.
func validateLiveKitEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("LIVEKIT_ENDPOINT invalid URL %q: %w", raw, err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "wss" {
		return fmt.Errorf("LIVEKIT_ENDPOINT must use https or wss scheme, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("LIVEKIT_ENDPOINT must include a host")
	}
	return nil
}

// LoadAudioPolicy reads the Opus bitrate override. The audio-processing
// flags (noise suppression, automatic gain control, echo cancellation) are
// constitution-fixed (§V): DefaultAudioPolicy is their only source, and no
// environment variable may enable them. The Opus bitrate may only be raised,
// never lowered below MinLiveKitOpusBitrateKbps.
func LoadAudioPolicy() (AudioPolicy, error) {
	return loadAudioPolicy(os.Getenv)
}

func loadAudioPolicy(getenv func(string) string) (AudioPolicy, error) {
	cfg := DefaultAudioPolicy()

	var err error
	cfg.OpusBitrateKbps, err = envInt(getenv, "LIVEKIT_AUDIO_OPUS_BITRATE_KBPS", cfg.OpusBitrateKbps)
	if err != nil {
		return AudioPolicy{}, err
	}
	if cfg.OpusBitrateKbps < MinLiveKitOpusBitrateKbps {
		return AudioPolicy{}, fmt.Errorf(
			"LIVEKIT_AUDIO_OPUS_BITRATE_KBPS must be >= %d (constitution §V), got %d",
			MinLiveKitOpusBitrateKbps, cfg.OpusBitrateKbps,
		)
	}
	return cfg, nil
}
