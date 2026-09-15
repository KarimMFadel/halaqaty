//go:build contract

package contract

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChatDependencyBoundaryDocuments proves that F-004's published boundary
// consumes existing identity/session, membership, and realtime transport
// ownership without taking ownership of invitations, media transport, or
// background notifications.
func TestChatDependencyBoundaryDocuments(t *testing.T) {
	root := repositoryRoot(t)
	documents := map[string]string{
		"spec":               filepath.Join(root, "specs", "004-real-time-chat", "spec.md"),
		"plan":               filepath.Join(root, "specs", "004-real-time-chat", "plan.md"),
		"websocket contract": filepath.Join(root, "docs", "contracts", "ws_events.md"),
		"chat ADR":           filepath.Join(root, "docs", "engineering", "architecture", "adr", "ADR-021-chat-persistence-media-and-delivery.md"),
	}

	required := map[string][]string{
		"spec": {
			"F-001 continues to own Firebase identity and current-device backend sessions",
			"F-002 continues to own circle membership, roles, invitations, removal, and archive state",
			"F-005 continues to own the generic realtime ticket and topic transport",
			"current Firebase identity",
			"current-device backend session",
			"circle_members",
			"POST /api/v1/realtime/tickets",
		},
		"plan": {
			"F-001 authentication/session checks",
			"F-002 memberships",
			"F-005 generic realtime transport",
			"circle_members",
			"POST /api/v1/realtime/tickets",
			"circle.{uuid}",
		},
		"websocket contract": {
			"F-004 reuses this authenticated socket",
			"authorized `circle.{circle_id}` topics",
			"POST /api/v1/realtime/tickets",
		},
		"chat ADR": {
			"membership-period history",
			"circle_members.joined_at",
			"existing F-005 WebSocket hub",
		},
	}
	for name, path := range documents {
		contents := readChatBoundaryDocument(t, path)
		for _, marker := range required[name] {
			if !strings.Contains(contents, marker) {
				t.Errorf("%s must document chat boundary marker %q", name, marker)
			}
		}
	}

	spec := readChatBoundaryDocument(t, documents["spec"])
	for _, marker := range []string{
		"Implementing the ADR-010 invitation amendment inside F-004; F-004 consumes the active memberships produced by F-002.",
		"F-004 MUST NOT create background-notification triggers or implement FCM token management",
		"MUST NOT require or grant access to a live-session topic or media room",
	} {
		if !strings.Contains(spec, marker) {
			t.Errorf("spec must preserve exclusion %q", marker)
		}
	}

	ws := readChatBoundaryDocument(t, documents["websocket contract"])
	for _, marker := range []string{
		"LiveKit remains audio-only",
		"F-004 emits no Firebase/FCM trigger",
		"F-008 owns all background and closed-app notifications",
	} {
		if !strings.Contains(ws, marker) {
			t.Errorf("websocket contract must preserve exclusion %q", marker)
		}
	}
}

// TestChatDependencyBoundaryRuntime scans the backend chat package when it is
// present. This keeps the contract gate useful before implementation while
// failing as soon as chat runtime code imports an owned provider or implements
// an excluded invitation/notification boundary.
func TestChatDependencyBoundaryRuntime(t *testing.T) {
	root := repositoryRoot(t)
	chatRoot := filepath.Join(root, "backend", "internal", "chat")
	if _, err := os.Stat(chatRoot); os.IsNotExist(err) {
		t.Log("backend/internal/chat is not implemented yet; documentation boundary remains enforced")
		return
	} else if err != nil {
		t.Fatalf("stat chat package: %v", err)
	}

	seen := make(map[string]bool)
	err := filepath.Walk(chatRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			seen[importPath] = true
			if chatForbiddenImport(importPath) {
				t.Errorf("chat runtime imports excluded dependency %q in %s", importPath, filepath.ToSlash(path))
			}
		}

		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if chatForbiddenSource(path, string(contents)) {
			t.Errorf("chat runtime contains excluded invitation implementation in %s", filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk chat package: %v", err)
	}

	// The foundational package is intentionally created before service,
	// authorization, and transport wiring. Keep forbidden-import checks active
	// immediately, but defer positive import checks until a wiring file exists;
	// otherwise adding models.go would incorrectly fail the Phase 2 gate.
	if chatHasWiringFile(chatRoot) {
		for name, prefixes := range map[string][]string{
			"F-001 identity/session":   {"/internal/auth", "/internal/middleware"},
			"F-002 membership/roles":   {"/internal/rbac"},
			"F-005 realtime transport": {"/internal/realtime"},
		} {
			if !hasImportWithPrefix(seen, prefixes) {
				t.Errorf("chat runtime must consume %s once wiring is present", name)
			}
		}
	} else {
		t.Log("chat wiring is not implemented yet; positive runtime dependency checks remain deferred")
	}
}

func readChatBoundaryDocument(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chat boundary document %q: %v", path, err)
	}
	return string(contents)
}

func chatForbiddenImport(importPath string) bool {
	return strings.Contains(importPath, "github.com/livekit/") ||
		strings.HasSuffix(importPath, "/internal/sessions/livekit") ||
		importPath == "firebase.google.com/go/messaging"
}

func chatForbiddenSource(path, contents string) bool {
	name := strings.ToLower(filepath.Base(path))
	lower := strings.ToLower(contents)
	return strings.Contains(name, "invite") ||
		strings.Contains(lower, "roleboundinvitation") ||
		strings.Contains(lower, "acceptinvitation") ||
		strings.Contains(lower, "createinvitation") ||
		strings.Contains(lower, "firebase messaging") ||
		strings.Contains(lower, "notification trigger")
}

func hasImportWithPrefix(imports map[string]bool, prefixes []string) bool {
	for imported := range imports {
		for _, prefix := range prefixes {
			if strings.Contains(imported, prefix) {
				return true
			}
		}
	}
	return false
}

func chatHasWiringFile(root string) bool {
	for _, name := range []string{"authorizer.go", "service.go", "handler.go", "realtime_projector.go"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}
