package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Bounding constants for chat-media configuration (ADR-021). Presigned GETs
// are versionless, so the TTL ceiling only bounds how long an unrevoked URL
// lives; the operation timeout bounds every MinIO call.
const (
	// MinChatMediaPresignTTL is the shortest allowed presigned-GET lifetime.
	MinChatMediaPresignTTL = time.Hour
	// MaxChatMediaPresignTTL is the longest allowed presigned-GET lifetime
	// (the SigV4 presign ceiling of seven days).
	MaxChatMediaPresignTTL = 7 * 24 * time.Hour
	// MinChatMediaOperationTimeout is the shortest allowed object-store
	// operation timeout.
	MinChatMediaOperationTimeout = time.Second
	// MaxChatMediaOperationTimeout is the longest allowed object-store
	// operation timeout.
	MaxChatMediaOperationTimeout = 120 * time.Second
)

// DefaultChatMediaOperationTimeout is the production-safe per-operation
// timeout applied when CHAT_MEDIA_OPERATION_TIMEOUT is unset.
const DefaultChatMediaOperationTimeout = 60 * time.Second

// ChatMediaConfig holds the validated private chat object-store settings.
// SecretAccessKey is a credential: String and GoString redact it so default
// formatting (fmt %v/%+v/%#v, slog) stays safe.
type ChatMediaConfig struct {
	Endpoint         string
	UseSSL           bool
	AccessKeyID      string
	SecretAccessKey  string
	Bucket           string
	PresignTTL       time.Duration
	OperationTimeout time.Duration
}

// String returns a log-safe representation with the secret access key
// redacted.
func (c ChatMediaConfig) String() string {
	return fmt.Sprintf(
		"ChatMediaConfig{Endpoint:%q UseSSL:%t AccessKeyID:%q SecretAccessKey:[REDACTED] Bucket:%q PresignTTL:%s OperationTimeout:%s}",
		c.Endpoint, c.UseSSL, c.AccessKeyID, c.Bucket, c.PresignTTL, c.OperationTimeout,
	)
}

// GoString returns the same redacted representation so %#v formatting cannot
// leak the secret access key.
func (c ChatMediaConfig) GoString() string { return c.String() }

// LoadChatMediaConfig reads and validates the chat-media object-store
// settings from the environment. All values are optional as a set: when none
// is set the zero config is returned (chat media disabled). When any value is
// set, endpoint, access key, secret, and bucket are all required. The
// endpoint is host[:port] without scheme; TLS is selected by CHAT_MEDIA_USE_SSL.
func LoadChatMediaConfig() (ChatMediaConfig, error) {
	return loadChatMediaConfig(os.Getenv)
}

func loadChatMediaConfig(getenv func(string) string) (ChatMediaConfig, error) {
	endpoint := strings.TrimSpace(getenv("CHAT_MEDIA_ENDPOINT"))
	accessKeyID := strings.TrimSpace(getenv("CHAT_MEDIA_ACCESS_KEY_ID"))
	secret := getenv("CHAT_MEDIA_SECRET_ACCESS_KEY")
	bucket := strings.TrimSpace(getenv("CHAT_MEDIA_BUCKET"))

	coreSet := endpoint != "" || accessKeyID != "" || secret != "" || bucket != ""
	optSet := getenv("CHAT_MEDIA_USE_SSL") != "" ||
		getenv("CHAT_MEDIA_PRESIGN_TTL") != "" ||
		getenv("CHAT_MEDIA_OPERATION_TIMEOUT") != ""
	if !coreSet && !optSet {
		return ChatMediaConfig{}, nil
	}
	if endpoint == "" {
		return ChatMediaConfig{}, fmt.Errorf("CHAT_MEDIA_ENDPOINT is required when chat media is configured")
	}
	if strings.Contains(endpoint, "://") {
		return ChatMediaConfig{}, fmt.Errorf("CHAT_MEDIA_ENDPOINT must be host[:port] without scheme, got %q", endpoint)
	}
	if accessKeyID == "" {
		return ChatMediaConfig{}, fmt.Errorf("CHAT_MEDIA_ACCESS_KEY_ID is required when chat media is configured")
	}
	if secret == "" {
		return ChatMediaConfig{}, fmt.Errorf("CHAT_MEDIA_SECRET_ACCESS_KEY is required when chat media is configured")
	}
	if bucket == "" {
		return ChatMediaConfig{}, fmt.Errorf("CHAT_MEDIA_BUCKET is required when chat media is configured")
	}

	cfg := ChatMediaConfig{
		Endpoint:         endpoint,
		AccessKeyID:      accessKeyID,
		SecretAccessKey:  secret,
		Bucket:           bucket,
		PresignTTL:       MaxChatMediaPresignTTL,
		OperationTimeout: DefaultChatMediaOperationTimeout,
	}

	var err error
	cfg.UseSSL, err = envBool(getenv, "CHAT_MEDIA_USE_SSL", false)
	if err != nil {
		return ChatMediaConfig{}, err
	}
	cfg.PresignTTL, err = envDuration(getenv, "CHAT_MEDIA_PRESIGN_TTL", cfg.PresignTTL)
	if err != nil {
		return ChatMediaConfig{}, err
	}
	if cfg.PresignTTL < MinChatMediaPresignTTL || cfg.PresignTTL > MaxChatMediaPresignTTL {
		return ChatMediaConfig{}, fmt.Errorf(
			"CHAT_MEDIA_PRESIGN_TTL must be between %s and %s",
			MinChatMediaPresignTTL, MaxChatMediaPresignTTL,
		)
	}
	cfg.OperationTimeout, err = envDuration(getenv, "CHAT_MEDIA_OPERATION_TIMEOUT", cfg.OperationTimeout)
	if err != nil {
		return ChatMediaConfig{}, err
	}
	if cfg.OperationTimeout < MinChatMediaOperationTimeout || cfg.OperationTimeout > MaxChatMediaOperationTimeout {
		return ChatMediaConfig{}, fmt.Errorf(
			"CHAT_MEDIA_OPERATION_TIMEOUT must be between %s and %s",
			MinChatMediaOperationTimeout, MaxChatMediaOperationTimeout,
		)
	}
	return cfg, nil
}

func envBool(getenv func(string) string, key string, fallback bool) (bool, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s invalid boolean %q: %w", key, raw, err)
	}
	return value, nil
}
