package auth

import (
	"context"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
)

func TestNoopAuditLogger_Log_DoesNotPanic(t *testing.T) {
	// The noop logger is selected by NewService when no audit logger is
	// supplied. This guards against a future change that accidentally adds
	// side effects (or a panic) to the discard implementation.
	var audit noopAuditLogger
	audit.Log(context.Background(), logging.RegisterEvent("user-id", "user@example.com"))
}

func TestNewService_WithNilAudit_UsesNoopLogger(t *testing.T) {
	store := &stubServiceStore{}
	svc := NewService(store, nil, 0)
	if svc.audit == nil {
		t.Fatal("NewService must replace nil audit with noop logger")
	}
}

type stubServiceStore struct{}

func (stubServiceStore) UpsertUserByFirebaseUID(context.Context, string, string) (User, bool, error) {
	return User{ID: "user-id"}, true, nil
}
func (stubServiceStore) UpsertProfileOnRegister(context.Context, string, string, string) error {
	return nil
}
func (stubServiceStore) GetUserProfileByUserID(context.Context, string) (UserProfile, error) {
	return UserProfile{ID: "user-id"}, nil
}
func (stubServiceStore) CreateSession(context.Context, Session) error    { return nil }
func (stubServiceStore) Revoke(context.Context, string, time.Time) error { return nil }
