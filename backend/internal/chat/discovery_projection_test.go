package chat

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDiscoveryMessageProjectionsKeepReplyAndPinMetadataOutOfWebSocket(t *testing.T) {
	pinnedAt := time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	replyID := uuid.New()
	pinnerID := uuid.New()
	message := Message{
		ID:        uuid.New(),
		SenderID:  uuid.New(),
		Type:      MessageTypeText,
		SentAt:    pinnedAt,
		ReplyToID: &replyID,
		ReplyPreview: &ReplyPreview{
			ID:      replyID,
			Deleted: true,
		},
		PinnedBy: &pinnerID,
		PinnedAt: &pinnedAt,
	}

	rest := newMessageResponse(message)
	if rest.ReplyToID != replyID.String() || rest.ReplyPreview == nil || !rest.ReplyPreview.Deleted || rest.ReplyPreview.Preview != "" || rest.PinnedBy != pinnerID.String() || rest.PinnedAt != pinnedAt.Format(time.RFC3339Nano) {
		t.Fatalf("REST projection = %#v", rest)
	}
	ws := chatMessagePayload(message)
	for _, key := range []string{"reply_to_id", "reply_preview", "pinned_by", "pinned_at"} {
		if _, present := ws[key]; present {
			t.Fatalf("WebSocket projection must omit %q: %#v", key, ws)
		}
	}
}
