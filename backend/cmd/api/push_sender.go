package main

import (
	"context"
	"fmt"

	"firebase.google.com/go/v4/messaging"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

// fcmMessageSender is the narrow send seam over the Firebase Admin messaging
// client; *messaging.Client satisfies it.
type fcmMessageSender interface {
	Send(ctx context.Context, message *messaging.Message) (string, error)
}

// fcmPushSender adapts the Firebase Admin messaging client to
// notifications.PushSender.
type fcmPushSender struct {
	client fcmMessageSender
}

var _ notifications.PushSender = (*fcmPushSender)(nil)

// auth.SessionRepository satisfies the dispatcher's token-removal seam.
var _ notifications.DeviceTokenRemover = (*auth.SessionRepository)(nil)

// newFCMPushSender wraps the Admin messaging client as a PushSender.
func newFCMPushSender(client fcmMessageSender) *fcmPushSender {
	return &fcmPushSender{client: client}
}

// Send delivers one lock-screen-safe projection. The payload carries only the
// generic title/body and minimal typed data (type, notification id, typed
// source reference) — never message bodies, grade details, media/room data,
// or raw tokens beyond FCM's required target field. An unregistered token
// maps to a wrapped notifications.ErrInvalidDeviceToken so the dispatcher
// deletes it through the F-001 seam and never retries; every other failure is
// transient and retried with bounded backoff.
func (s *fcmPushSender) Send(ctx context.Context, message notifications.PushMessage) error {
	_, err := s.client.Send(ctx, &messaging.Message{
		Token: message.Token,
		Notification: &messaging.Notification{
			Title: message.Title,
			Body:  message.Body,
		},
		Data: map[string]string{
			"type":            string(message.Type),
			"notification_id": message.NotificationID.String(),
			"source_ref_type": string(message.SourceRef.Type),
			"source_ref_id":   message.SourceRef.ID.String(),
		},
	})
	if err == nil {
		return nil
	}
	if messaging.IsUnregistered(err) {
		return fmt.Errorf("fcm send rejected the registration token: %w", notifications.ErrInvalidDeviceToken)
	}
	// Deliberate: INVALID_ARGUMENT stays transient. A 400 can stem from the
	// payload rather than the token, and wrongly deleting a valid token is
	// worse than parking after five attempts — only an explicit UNREGISTERED
	// response proves the token is dead (ADR-027 token invalidation).
	return fmt.Errorf("fcm send: %w", err)
}
