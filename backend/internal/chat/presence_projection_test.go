package chat

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewMessageResponse_ExposesSenderReadDetailsAndReadHeadline(t *testing.T) {
	readAt := time.Date(2026, 9, 3, 12, 2, 0, 0, time.UTC)
	message := Message{
		ID:           uuid.New(),
		SenderID:     uuid.New(),
		Type:         MessageTypeText,
		SentAt:       readAt.Add(-time.Minute),
		ReadReceipts: []MessageRead{{UserID: uuid.New(), ReadAt: readAt}},
	}

	response := newMessageResponse(message)
	if response.DeliveryStatus != string(DeliveryStatusRead) {
		t.Fatalf("delivery status = %q, want %q", response.DeliveryStatus, DeliveryStatusRead)
	}
	if len(response.ReadReceipts) != 1 || response.ReadReceipts[0].ReaderID != message.ReadReceipts[0].UserID.String() || response.ReadReceipts[0].ReadAt != readAt.Format(time.RFC3339Nano) {
		t.Fatalf("read receipts = %#v", response.ReadReceipts)
	}
}
