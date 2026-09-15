//go:build contract

package contract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChatArchitectureBoundary_ChatPackagesStayWithinApprovedDependencies is a
// supplemental source-policy guard for T089. Runtime tests cannot prove an
// absent import, so it limits this check to chat-owned production packages.
func TestChatArchitectureBoundary_ChatPackagesStayWithinApprovedDependencies(t *testing.T) {
	for _, directory := range []string{"../../internal/chat", "../../internal/realtime"} {
		assertNoForbiddenChatImports(t, directory, []string{
			"livekit",
			"firebase.google.com/go/messaging",
			"firebase.google.com/go/v4/messaging",
		})
	}
	assertRealtimeOwnsTheOnlyWebSocketTransport(t, "../../internal/realtime")
	assertNoGlobalRoleSchema(t, "../../migrations/000018_real_time_chat.up.sql")
}

func assertNoForbiddenChatImports(t *testing.T, directory string, forbidden []string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("list %s: %v", directory, err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse imports in %s: %v", path, parseErr)
		}
		for _, imported := range parsed.Imports {
			assertImportIsAllowed(t, path, imported, forbidden)
		}
	}
}

func assertRealtimeOwnsTheOnlyWebSocketTransport(t *testing.T, directory string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("list %s: %v", directory, err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "hub.go" {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse imports in %s: %v", path, parseErr)
		}
		for _, imported := range parsed.Imports {
			if strings.Contains(strings.Trim(imported.Path.Value, "\""), "gorilla/websocket") {
				t.Errorf("%s adds a second WebSocket transport; only realtime/hub.go may own it", path)
			}
		}
	}
}

func assertImportIsAllowed(t *testing.T, path string, imported *ast.ImportSpec, forbidden []string) {
	t.Helper()
	value := strings.Trim(imported.Path.Value, "\"")
	for _, blocked := range forbidden {
		if strings.Contains(value, blocked) {
			t.Errorf("%s imports forbidden chat dependency %q", path, value)
		}
	}
}

func assertNoGlobalRoleSchema(t *testing.T, path string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chat migration %s: %v", path, err)
	}
	if strings.Contains(strings.ToLower(string(contents)), "global_role") {
		t.Errorf("%s introduces a forbidden global role", path)
	}
}
