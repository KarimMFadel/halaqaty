package notifications

import (
	"context"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/google/uuid"
)

// userEventSender is the narrow realtime seam a Notifier needs: delivering an
// already-redacted event to one user's authenticated connections.
type userEventSender interface {
	SendToUsers(ctx context.Context, userIDs []string, delivery realtime.AuthorizedDelivery) error
}

// Compile-time proof that the shared hub satisfies the notifier seam.
var _ userEventSender = (*realtime.Hub)(nil)

// Notifier publishes lock-screen-safe notification.updated hints on the
// realtime transport. Hints are best-effort: REST history stays authoritative
// and delivery may be duplicated or missed, so events carry no dedupe id.
type Notifier struct {
	sender userEventSender
}

// NewNotifier constructs a realtime notifier. A nil sender disables
// publication; NotifyReadState then becomes a no-op.
func NewNotifier(sender userEventSender) *Notifier {
	return &Notifier{sender: sender}
}

// NotifyReadState publishes the authoritative unread count after a read-state
// change commits. notificationID is set for single-notification changes and
// nil for bulk read-all updates, per the realtime contract.
func (n *Notifier) NotifyReadState(ctx context.Context, userID string, notificationID *uuid.UUID, unreadCount int64) error {
	if n == nil || n.sender == nil || userID == "" {
		return nil
	}

	payload := map[string]any{"unread_count": unreadCount}
	if notificationID != nil {
		payload["notification_id"] = notificationID.String()
	}
	envelope := map[string]any{
		"type":      realtime.EventNotificationUpdated,
		"payload":   payload,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}

	delivery := realtime.AuthorizedDelivery{
		EventID: "",
		Payload: envelope,
		Authorize: func(_ context.Context, connection realtime.ConnectionIdentity) (bool, error) {
			return connection.UserID == userID, nil
		},
	}
	return n.sender.SendToUsers(ctx, []string{userID}, delivery)
}
