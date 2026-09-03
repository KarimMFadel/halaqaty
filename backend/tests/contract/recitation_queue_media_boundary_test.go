//go:build contract

package contract

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// TestRecitationQueueMediaBoundary uses the compiler's package dependency
// graph, not a source-text scan, to enforce the provider ownership seam.
func TestRecitationQueueMediaBoundary(t *testing.T) {
	backendRoot := filepath.Join("..", "..")
	command := exec.Command("go", "list", "-f", "{{.ImportPath}}|{{join .Imports \" \"}}", "./internal/...")
	command.Dir = backendRoot
	output, err := command.Output()
	if err != nil {
		t.Fatalf("read compiled internal dependency graph: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			t.Fatalf("unexpected go list output %q", line)
		}
		imports := strings.Fields(parts[1])
		for _, imported := range imports {
			if strings.Contains(imported, "livekit") && parts[0] != "github.com/KarimMFadel/halaqaty/backend/internal/sessions/livekit" {
				t.Fatalf("provider dependency %q escapes sessions/livekit through %q", imported, parts[0])
			}
			if parts[0] == "github.com/KarimMFadel/halaqaty/backend/internal/queue" && imported == "github.com/KarimMFadel/halaqaty/backend/internal/sessions" {
				t.Fatalf("queue imports sessions media boundary")
			}
		}
	}
}

func TestRecitationQueueMediaBoundary_AuthorizedStudentKeepsAudioOnlyGrant(t *testing.T) {
	store := newSessionStoreStub()
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID: {scTeacher: "teacher", scStudent: "student"},
	}}
	gateway := newPhase6RecordingGateway()
	service := newLiveSessionContractService(store, gateway, roles)

	created, err := service.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), scTeacher, created.ID); err != nil {
		t.Fatalf("start session: %v", err)
	}
	if _, _, err := service.JoinSession(context.Background(), scStudent, created.ID); err != nil {
		t.Fatalf("join student: %v", err)
	}
	if len(gateway.grants) != 2 {
		t.Fatalf("connection grants=%d, want start and student join", len(gateway.grants))
	}
	for _, grants := range gateway.grants {
		if !grants.CanPublishAudio {
			t.Fatal("authorized participant was not granted audio publishing")
		}
	}
}

type phase6RecordingGateway struct{ grants []sessions.MediaGrants }

func newPhase6RecordingGateway() *phase6RecordingGateway { return new(phase6RecordingGateway) }

func (*phase6RecordingGateway) EnsureRoom(context.Context, sessions.MediaRoomRef, sessions.MediaMode) error {
	return nil
}
func (*phase6RecordingGateway) CloseRoom(context.Context, sessions.MediaRoomRef) error { return nil }
func (*phase6RecordingGateway) MuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (*phase6RecordingGateway) UnmuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (*phase6RecordingGateway) MuteAll(context.Context, sessions.MediaRoomRef) error { return nil }
func (*phase6RecordingGateway) RemoveParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (g *phase6RecordingGateway) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, userID string, grants sessions.MediaGrants) (sessions.MediaConnection, error) {
	g.grants = append(g.grants, grants)
	return sessions.MediaConnection{Endpoint: "wss://media.test", Credential: sessions.MediaCredential("phase6-" + userID), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

var _ sessions.SessionMediaGateway = (*phase6RecordingGateway)(nil)
