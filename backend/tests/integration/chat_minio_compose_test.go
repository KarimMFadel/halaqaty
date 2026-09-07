//go:build integration

// These tests are bounded policy guards over three declarative artifacts
// (docker/minio.Dockerfile, docker-compose.yml, .env.example) required by
// Spec-Kit task T013: the file content IS the deliverable being verified
// (pinned source tag, secret-free placeholders, healthcheck, volume,
// versioning init). They supplement, not replace, the behavioral coverage in
// backend/internal/platform/config and backend/internal/chat tests.

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// chatMediaRepoPath resolves a repo-root-relative path from this test file's
// location (backend/tests/integration), following the runMigrationFile
// runtime.Caller pattern so the test works from any working directory.
func chatMediaRepoPath(t *testing.T, elements ...string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	return filepath.Join(append([]string{filepath.Dir(currentFile), "..", "..", ".."}, elements...)...)
}

func chatMediaReadFile(t *testing.T, elements ...string) string {
	t.Helper()
	raw, err := os.ReadFile(chatMediaRepoPath(t, elements...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(elements...), err)
	}
	return string(raw)
}

// composeServiceBlock extracts one top-level service's block from the compose
// file by indentation: from the "  <service>:" key until the next service or
// top-level key. The compose file is project-owned and two-space indented.
func composeServiceBlock(t *testing.T, compose, service string) string {
	t.Helper()
	var block []string
	inBlock := false
	for _, line := range strings.Split(compose, "\n") {
		if strings.HasPrefix(line, "  "+service+":") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if trimmed := strings.TrimRight(line, " \t\r"); trimmed == "" {
			block = append(block, line)
			continue
		}
		// Service body keys live at four-space indent; a shallower key ends
		// the block.
		if !strings.HasPrefix(line, "    ") {
			break
		}
		block = append(block, line)
	}
	if len(block) == 0 {
		t.Fatalf("compose service %q not found", service)
	}
	return strings.Join(block, "\n")
}

// envExampleValue returns the value of one KEY=value line in .env.example.
func envExampleValue(t *testing.T, content, key string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if after, ok := strings.CutPrefix(line, key+"="); ok {
			return strings.TrimSpace(strings.TrimRight(after, "\r"))
		}
	}
	t.Fatalf(".env.example is missing %s", key)
	return ""
}

func TestChatMinioDockerfile_BuildsFromPinnedOfficialSourceTag(t *testing.T) {
	dockerfile := chatMediaReadFile(t, "docker", "minio.Dockerfile")

	if !strings.Contains(dockerfile, "RELEASE.2025-10-15T17-29-55Z") {
		t.Fatal("Dockerfile must contain the literal official source tag RELEASE.2025-10-15T17-29-55Z")
	}
	if !strings.Contains(dockerfile, "https://github.com/minio/minio") {
		t.Fatal("Dockerfile must build from the official MinIO source repository")
	}
	if stages := strings.Count(dockerfile, "FROM "); stages < 2 {
		t.Fatalf("Dockerfile must be a multi-stage build, found %d FROM stages", stages)
	}
}

func TestChatMinioCompose_ServiceVolumeHealthcheckAndBucketInit(t *testing.T) {
	compose := chatMediaReadFile(t, "docker-compose.yml")

	minio := composeServiceBlock(t, compose, "minio")
	for _, fragment := range []string{
		"dockerfile: docker/minio.Dockerfile",
		"image: halaqaty-minio:local",
		"halaqaty-minio-data:/data",
		"healthcheck:",
		"MINIO_ROOT_USER: ${MINIO_ROOT_USER:-",
		"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-",
	} {
		if !strings.Contains(minio, fragment) {
			t.Fatalf("compose minio service missing %q:\n%s", fragment, minio)
		}
	}

	if !strings.Contains(compose, "  halaqaty-minio-data:") {
		t.Fatal("compose must declare the persistent halaqaty-minio-data volume at top level")
	}

	init := composeServiceBlock(t, compose, "minio-init")
	for _, fragment := range []string{
		"condition: service_healthy",
		"mc mb --ignore-existing",
		"mc versioning enable",
		"CHAT_MEDIA_BUCKET: ${CHAT_MEDIA_BUCKET:-halaqaty-chat}",
	} {
		if !strings.Contains(init, fragment) {
			t.Fatalf("compose minio-init service missing %q:\n%s", fragment, init)
		}
	}
}

func TestChatMinioEnvExample_PlaceholdersOnlyAndBoundedDefaults(t *testing.T) {
	content := chatMediaReadFile(t, ".env.example")

	required := []struct {
		key  string
		want string
	}{
		{key: "MINIO_ROOT_USER", want: "minioadmin"},
		{key: "MINIO_ROOT_PASSWORD", want: "minioadmin"},
		{key: "MINIO_API_PORT", want: "9000"},
		{key: "MINIO_CONSOLE_PORT", want: "9001"},
		{key: "CHAT_MEDIA_ENDPOINT", want: "localhost:9000"},
		{key: "CHAT_MEDIA_USE_SSL", want: "false"},
		{key: "CHAT_MEDIA_ACCESS_KEY_ID", want: "minioadmin"},
		{key: "CHAT_MEDIA_SECRET_ACCESS_KEY", want: ""},
		{key: "CHAT_MEDIA_BUCKET", want: "halaqaty-chat"},
		{key: "CHAT_MEDIA_PRESIGN_TTL", want: "168h"},
		{key: "CHAT_MEDIA_OPERATION_TIMEOUT", want: "60s"},
	}
	for _, tc := range required {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			value := envExampleValue(t, content, tc.key)
			if value != tc.want {
				t.Fatalf("%s: got %q, want %q", tc.key, value, tc.want)
			}
			// Placeholder guard for the gitleaks gate: no shipped value may
			// look like a real credential.
			if len(value) > 24 {
				t.Fatalf("%s ships a high-entropy value (len %d); placeholder values only", tc.key, len(value))
			}
		})
	}
}
