package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLoadChatMediaConfig_EmptyWhenUnset(t *testing.T) {
	cfg, err := loadChatMediaConfig(func(string) string { return "" })

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if cfg != (ChatMediaConfig{}) {
		t.Fatalf("config: got %+v, want zero value", cfg)
	}
}

func TestLoadChatMediaConfig_ValidMinimalEnvAppliesDefaults(t *testing.T) {
	env := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "localhost:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
	}

	cfg, err := loadChatMediaConfig(func(key string) string { return env[key] })

	if err != nil {
		t.Fatalf("loadChatMediaConfig: %v", err)
	}
	if cfg.Endpoint != "localhost:9000" || cfg.AccessKeyID != "minioadmin" || cfg.Bucket != "halaqaty-chat" {
		t.Fatalf("core fields: got %+v", cfg)
	}
	if cfg.SecretAccessKey != "object-store-secret" {
		t.Fatalf("secret: got %q", cfg.SecretAccessKey)
	}
	if cfg.UseSSL {
		t.Fatal("UseSSL default: got true, want false")
	}
	if cfg.PresignTTL != 7*24*time.Hour {
		t.Fatalf("PresignTTL default: got %v, want %v", cfg.PresignTTL, 7*24*time.Hour)
	}
	if cfg.OperationTimeout != 60*time.Second {
		t.Fatalf("OperationTimeout default: got %v, want %v", cfg.OperationTimeout, 60*time.Second)
	}
}

func TestLoadChatMediaConfig_AppliesOverrides(t *testing.T) {
	env := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "minio.internal:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "svc-chat",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "chat-prod",
		"CHAT_MEDIA_USE_SSL":           "true",
		"CHAT_MEDIA_PRESIGN_TTL":       "24h",
		"CHAT_MEDIA_OPERATION_TIMEOUT": "30s",
	}

	cfg, err := loadChatMediaConfig(func(key string) string { return env[key] })

	if err != nil {
		t.Fatalf("loadChatMediaConfig: %v", err)
	}
	if !cfg.UseSSL {
		t.Fatal("UseSSL override: got false, want true")
	}
	if cfg.PresignTTL != 24*time.Hour {
		t.Fatalf("PresignTTL override: got %v, want 24h", cfg.PresignTTL)
	}
	if cfg.OperationTimeout != 30*time.Second {
		t.Fatalf("OperationTimeout override: got %v, want 30s", cfg.OperationTimeout)
	}
}

func TestLoadChatMediaConfig_RejectsMissingRequiredValues(t *testing.T) {
	base := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "localhost:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
	}
	cases := []struct {
		name   string
		key    string
		value  string
		wantIn string
	}{
		{name: "endpoint unset", key: "CHAT_MEDIA_ENDPOINT", value: "", wantIn: "CHAT_MEDIA_ENDPOINT is required"},
		{name: "endpoint blank", key: "CHAT_MEDIA_ENDPOINT", value: "   ", wantIn: "CHAT_MEDIA_ENDPOINT is required"},
		{name: "access key unset", key: "CHAT_MEDIA_ACCESS_KEY_ID", value: "", wantIn: "CHAT_MEDIA_ACCESS_KEY_ID is required"},
		{name: "secret unset", key: "CHAT_MEDIA_SECRET_ACCESS_KEY", value: "", wantIn: "CHAT_MEDIA_SECRET_ACCESS_KEY is required"},
		{name: "bucket unset", key: "CHAT_MEDIA_BUCKET", value: "", wantIn: "CHAT_MEDIA_BUCKET is required"},
		{name: "bucket blank", key: "CHAT_MEDIA_BUCKET", value: " ", wantIn: "CHAT_MEDIA_BUCKET is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string, len(base))
			for k, v := range base {
				env[k] = v
			}
			env[tc.key] = tc.value

			_, err := loadChatMediaConfig(func(key string) string { return env[key] })

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tc.wantIn)
			}
		})
	}
}

func TestLoadChatMediaConfig_RejectsSchemedEndpoint(t *testing.T) {
	env := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "https://minio.internal:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
	}

	_, err := loadChatMediaConfig(func(key string) string { return env[key] })

	if err == nil {
		t.Fatal("error: got nil, want validation failure")
	}
	if !strings.Contains(err.Error(), "without scheme") {
		t.Fatalf("error: got %q, want it to reject a schemed endpoint", err.Error())
	}
}

func TestLoadChatMediaConfig_RejectsOutOfBoundsDurations(t *testing.T) {
	base := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "localhost:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
	}
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "presign ttl below minimum", key: "CHAT_MEDIA_PRESIGN_TTL", value: "30m"},
		{name: "presign ttl above maximum", key: "CHAT_MEDIA_PRESIGN_TTL", value: "169h"},
		{name: "operation timeout below minimum", key: "CHAT_MEDIA_OPERATION_TIMEOUT", value: "500ms"},
		{name: "operation timeout above maximum", key: "CHAT_MEDIA_OPERATION_TIMEOUT", value: "121s"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string, len(base))
			for k, v := range base {
				env[k] = v
			}
			env[tc.key] = tc.value

			_, err := loadChatMediaConfig(func(key string) string { return env[key] })

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
			if !strings.Contains(err.Error(), "must be between") {
				t.Fatalf("error: got %q, want it to contain the bounds message", err.Error())
			}
		})
	}
}

func TestLoadChatMediaConfig_RejectsInvalidValues(t *testing.T) {
	base := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "localhost:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "object-store-secret",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
	}
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "invalid duration", key: "CHAT_MEDIA_PRESIGN_TTL", value: "soon"},
		{name: "invalid boolean", key: "CHAT_MEDIA_USE_SSL", value: "maybe"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string, len(base))
			for k, v := range base {
				env[k] = v
			}
			env[tc.key] = tc.value

			_, err := loadChatMediaConfig(func(key string) string { return env[key] })

			if err == nil {
				t.Fatal("error: got nil, want parse failure")
			}
		})
	}
}

func TestLoadChatMediaConfig_ReadsEnvironment(t *testing.T) {
	t.Setenv("CHAT_MEDIA_ENDPOINT", "minio.example.com:9000")
	t.Setenv("CHAT_MEDIA_ACCESS_KEY_ID", "env-access")
	t.Setenv("CHAT_MEDIA_SECRET_ACCESS_KEY", "env-secret")
	t.Setenv("CHAT_MEDIA_BUCKET", "env-bucket")

	cfg, err := LoadChatMediaConfig()

	if err != nil {
		t.Fatalf("LoadChatMediaConfig: %v", err)
	}
	if cfg.Endpoint != "minio.example.com:9000" || cfg.Bucket != "env-bucket" {
		t.Fatalf("config: got %+v", cfg)
	}
}

// TestChatMediaConfig_SecretNeverFormatted guards the constitution §IV secret
// invariant: no formatting of the config may expose SecretAccessKey, so a
// future String()/GoString()/MarshalJSON change that includes it fails here.
func TestChatMediaConfig_SecretNeverFormatted(t *testing.T) {
	cfg := ChatMediaConfig{
		Endpoint:        "localhost:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "super-secret-do-not-print",
		Bucket:          "halaqaty-chat",
	}

	for _, rendered := range []string{
		fmt.Sprint(cfg),
		fmt.Sprintf("%v", cfg),
		fmt.Sprintf("%+v", cfg),
		fmt.Sprintf("%#v", cfg),
		cfg.String(),
		cfg.GoString(),
	} {
		if strings.Contains(rendered, cfg.SecretAccessKey) {
			t.Fatalf("formatted config leaks secret access key: %q", rendered)
		}
	}
}

// TestLoadChatMediaConfig_ErrorsNeverContainSecret guards that validation
// error messages never echo the secret value.
func TestLoadChatMediaConfig_ErrorsNeverContainSecret(t *testing.T) {
	env := map[string]string{
		"CHAT_MEDIA_ENDPOINT":          "localhost:9000",
		"CHAT_MEDIA_ACCESS_KEY_ID":     "minioadmin",
		"CHAT_MEDIA_SECRET_ACCESS_KEY": "super-secret-do-not-print",
		"CHAT_MEDIA_BUCKET":            "halaqaty-chat",
		"CHAT_MEDIA_PRESIGN_TTL":       "30m",
	}

	_, err := loadChatMediaConfig(func(key string) string { return env[key] })

	if err == nil {
		t.Fatal("error: got nil, want validation failure")
	}
	if strings.Contains(err.Error(), env["CHAT_MEDIA_SECRET_ACCESS_KEY"]) {
		t.Fatalf("error message leaks secret access key: %q", err.Error())
	}
}
