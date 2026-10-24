package notifications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/google/uuid"
)

type fakeUserEventSender struct {
	calls    int
	userIDs  []string
	delivery realtime.AuthorizedDelivery
	err      error
}

func (f *fakeUserEventSender) SendToUsers(_ context.Context, userIDs []string, delivery realtime.AuthorizedDelivery) error {
	f.calls++
	f.userIDs = userIDs
	f.delivery = delivery
	return f.err
}

func TestNotifyReadStatePublishesLockScreenSafeEnvelope(t *testing.T) {
	sender := &fakeUserEventSender{}
	notifier := notifications.NewNotifier(sender)
	userID := uuid.New().String()
	notificationID := uuid.New()

	before := time.Now().UTC()
	if err := notifier.NotifyReadState(context.Background(), userID, &notificationID, 2); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 || len(sender.userIDs) != 1 || sender.userIDs[0] != userID {
		t.Fatalf("SendToUsers calls=%d userIDs=%v, want one call for the recipient only", sender.calls, sender.userIDs)
	}
	// Delivery may be duplicated or missed, so no event id may request hub
	// dedupe.
	if sender.delivery.EventID != "" {
		t.Fatalf("EventID=%q, want empty", sender.delivery.EventID)
	}

	envelope, ok := sender.delivery.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type %T, want map envelope", sender.delivery.Payload)
	}
	if envelope["type"] != "notification.updated" {
		t.Fatalf("envelope type=%v, want notification.updated", envelope["type"])
	}
	rawTimestamp, ok := envelope["timestamp"].(string)
	if !ok {
		t.Fatalf("timestamp type %T, want string", envelope["timestamp"])
	}
	timestamp, err := time.Parse(time.RFC3339Nano, rawTimestamp)
	if err != nil || timestamp.Location() != time.UTC || timestamp.Before(before) {
		t.Fatalf("timestamp=%q parsed=%v err=%v, want recent UTC RFC3339", rawTimestamp, timestamp, err)
	}
	payload, ok := envelope["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type %T, want map", envelope["payload"])
	}
	if payload["notification_id"] != notificationID.String() {
		t.Fatalf("notification_id=%v, want %s", payload["notification_id"], notificationID)
	}
	if payload["unread_count"] != int64(2) {
		t.Fatalf("unread_count=%v, want 2", payload["unread_count"])
	}
	// The hint must stay lock-screen safe: no title, body, content, or
	// deep-link fields ever leave the server on the realtime channel.
	for _, key := range []string{"title", "body", "content", "source_ref", "deep_link", "token"} {
		if _, leaked := payload[key]; leaked {
			t.Fatalf("payload leaks %q", key)
		}
	}

	allowed, err := sender.delivery.Authorize(context.Background(), realtime.ConnectionIdentity{UserID: userID})
	if err != nil || !allowed {
		t.Fatalf("Authorize(recipient) = (%t, %v), want (true, nil)", allowed, err)
	}
	denied, err := sender.delivery.Authorize(context.Background(), realtime.ConnectionIdentity{UserID: uuid.New().String()})
	if err != nil || denied {
		t.Fatalf("Authorize(other) = (%t, %v), want (false, nil)", denied, err)
	}
}

func TestNotifyReadStateOmitsNotificationIDForBulkUpdates(t *testing.T) {
	sender := &fakeUserEventSender{}
	notifier := notifications.NewNotifier(sender)

	if err := notifier.NotifyReadState(context.Background(), uuid.New().String(), nil, 0); err != nil {
		t.Fatal(err)
	}
	envelope := sender.delivery.Payload.(map[string]any)
	payload := envelope["payload"].(map[string]any)
	if _, present := payload["notification_id"]; present {
		t.Fatalf("bulk payload %v must omit notification_id", payload)
	}
	if payload["unread_count"] != int64(0) {
		t.Fatalf("unread_count=%v, want 0", payload["unread_count"])
	}
}

func TestNotifyReadStateWithoutSenderIsANoOp(t *testing.T) {
	notifier := notifications.NewNotifier(nil)
	id := uuid.New()
	if err := notifier.NotifyReadState(context.Background(), uuid.New().String(), &id, 1); err != nil {
		t.Fatalf("nil sender NotifyReadState = %v, want nil", err)
	}
	var nilNotifier *notifications.Notifier
	if err := nilNotifier.NotifyReadState(context.Background(), uuid.New().String(), nil, 0); err != nil {
		t.Fatalf("nil notifier NotifyReadState = %v, want nil", err)
	}
}

func TestNotifyReadStatePropagatesSenderErrors(t *testing.T) {
	want := errors.New("hub unavailable")
	sender := &fakeUserEventSender{err: want}
	notifier := notifications.NewNotifier(sender)
	id := uuid.New()
	if err := notifier.NotifyReadState(context.Background(), uuid.New().String(), &id, 1); !errors.Is(err, want) {
		t.Fatalf("NotifyReadState error = %v, want %v", err, want)
	}
}
