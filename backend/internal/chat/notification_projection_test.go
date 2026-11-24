package chat

import (
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
)

func TestChatNotificationInputsAreGenericAndExpireFromMessageTime(t *testing.T) {
	senderID, recipientID, messageID := uuid.New(), uuid.New(), uuid.New()
	sentAt := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		direct bool
		want   notifications.EventType
	}{
		{name: "group", want: notifications.EventGroupMessage},
		{name: "private", direct: true, want: notifications.EventPrivateMessage},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := chatMessageNoticeInput("chat-event-8", recipientID, messageID, sentAt, test.direct)
			if input.Type != test.want || input.RecipientID != recipientID || input.SourceEventID != "chat-event-8" ||
				input.SourceRef.Type != notifications.SourceMessage || input.SourceRef.ID != messageID {
				t.Fatalf("chat notice identity = %+v", input)
			}
			if !input.PushExpiresAt.Equal(sentAt.Add(24*time.Hour)) || input.Title == "" || input.Body == "" ||
				input.Title == senderID.String() || input.Body == senderID.String() ||
				input.Title == recipientID.String() || input.Body == recipientID.String() {
				t.Fatalf("chat notice content/expiry = %+v", input)
			}
		})
	}
}
