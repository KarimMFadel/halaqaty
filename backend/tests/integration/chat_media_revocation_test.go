//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/google/uuid"
)

func TestChatMediaRevocation_ReconcilesLatestMarkerByMessageState(t *testing.T) {
	var reconciler *chat.MediaReconciler
	if reconciler == nil {
		t.Fatal("media reconciler is not configured")
	}
	_ = context.Background()
	_ = uuid.Nil
}
