package queue

import (
	"strings"
	"testing"
)

func TestTransitionEntryQueryGuardsPersistedStatus(t *testing.T) {
	if !strings.Contains(updateEntryTransitionQuery, "AND status = $3") {
		t.Fatal("transition update must guard the persisted status")
	}
}

func TestRoundSelectionQueryGuardsEntryRoundMembership(t *testing.T) {
	if !strings.Contains(updateRoundSelectionQuery, "e.queue_id = recitation_queue.id") {
		t.Fatal("round selection must verify the selected entry belongs to the round")
	}
}

func TestDeliveredOutboxQueryAllowsParkedReplay(t *testing.T) {
	if strings.Contains(markOutboxEventDeliveredQuery, "parked_at IS NULL") {
		t.Fatal("successful replay must be able to mark parked events delivered")
	}
}
