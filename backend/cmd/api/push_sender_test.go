package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	firebaseAdmin "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/google/uuid"
	"google.golang.org/api/option"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

// newTestMessagingClient builds a real Admin messaging client pointed at a
// local test server, so error classification is exercised against the SDK's
// own error types without any network or credential dependency.
func newTestMessagingClient(t *testing.T, handler http.HandlerFunc) *messaging.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	app, err := firebaseAdmin.NewApp(context.Background(),
		&firebaseAdmin.Config{ProjectID: "halaqaty-test"},
		option.WithEndpoint(server.URL),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client, err := app.Messaging(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func fcmErrorResponse(status int, errorCode string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":{"code":`+strconv.Itoa(status)+`,"message":"provider failure","status":"ERROR","details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"`+errorCode+`"}]}}`)
	}
}

func testPushMessage() notifications.PushMessage {
	return notifications.PushMessage{
		Token:          "test-registration-token",
		Title:          "New event",
		Body:           "Open the app to view it",
		NotificationID: uuid.New(),
		Type:           notifications.EventGroupMessage,
		SourceRef:      notifications.SourceRef{Type: notifications.SourceMessage, ID: uuid.New()},
	}
}

// T014: an unregistered-token provider response maps to a wrapped
// notifications.ErrInvalidDeviceToken so the dispatcher deletes the token
// through the F-001 seam and never retries it.
func TestFCMPushSenderMapsUnregisteredToInvalidDeviceToken(t *testing.T) {
	client := newTestMessagingClient(t, fcmErrorResponse(http.StatusNotFound, "UNREGISTERED"))
	sender := newFCMPushSender(client)
	err := sender.Send(context.Background(), testPushMessage())
	if !errors.Is(err, notifications.ErrInvalidDeviceToken) {
		t.Fatalf("Send error = %v, want wrapped ErrInvalidDeviceToken", err)
	}
}

// T014: quota and server-side provider failures are transient — the
// dispatcher retries them with bounded backoff instead of deleting the token.
func TestFCMPushSenderTreatsQuotaAndServerErrorsAsTransient(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"quota exceeded": fcmErrorResponse(http.StatusTooManyRequests, "QUOTA_EXCEEDED"),
		"unavailable":    fcmErrorResponse(http.StatusServiceUnavailable, "UNAVAILABLE"),
		"internal":       fcmErrorResponse(http.StatusInternalServerError, "INTERNAL"),
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestMessagingClient(t, handler)
			sender := newFCMPushSender(client)
			err := sender.Send(context.Background(), testPushMessage())
			if err == nil {
				t.Fatal("want a send error")
			}
			if errors.Is(err, notifications.ErrInvalidDeviceToken) {
				t.Fatalf("Send error = %v, want transient classification (token must survive)", err)
			}
		})
	}
}

// T014: the wire payload is the lock-screen-safe projection only — generic
// title/body plus minimal typed data; the raw token appears solely in FCM's
// required target field and never inside the data payload.
func TestFCMPushSenderSendsLockScreenSafeProjection(t *testing.T) {
	var payload struct {
		Message struct {
			Token        string            `json:"token"`
			Notification map[string]string `json:"notification"`
			Data         map[string]string `json:"data"`
		} `json:"message"`
	}
	client := newTestMessagingClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request %s: %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"projects/halaqaty-test/messages/1"}`)
	})
	sender := newFCMPushSender(client)
	message := testPushMessage()
	if err := sender.Send(context.Background(), message); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if payload.Message.Token != message.Token {
		t.Fatalf("target token = %q, want the registration token in the FCM target field", payload.Message.Token)
	}
	if payload.Message.Notification["title"] != message.Title || payload.Message.Notification["body"] != message.Body {
		t.Fatalf("notification projection = %v, want the generic title/body", payload.Message.Notification)
	}
	wantData := map[string]string{
		"type":            string(message.Type),
		"notification_id": message.NotificationID.String(),
		"source_ref_type": string(message.SourceRef.Type),
		"source_ref_id":   message.SourceRef.ID.String(),
	}
	for key, want := range wantData {
		if got := payload.Message.Data[key]; got != want {
			t.Fatalf("data[%q] = %q, want %q", key, got, want)
		}
	}
	if len(payload.Message.Data) != len(wantData) {
		t.Fatalf("data payload = %v, want exactly the minimal typed projection", payload.Message.Data)
	}
	for key, value := range payload.Message.Data {
		if value == message.Token || key == "token" {
			t.Fatalf("data payload leaks the raw token: %v", payload.Message.Data)
		}
	}
}

// T014: SDK-side validation failures (before any provider call) are transient
// send errors, never token invalidation.
func TestFCMPushSenderTreatsValidationFailureAsTransient(t *testing.T) {
	client := newTestMessagingClient(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("no provider call expected for an invalid message")
	})
	sender := newFCMPushSender(client)
	message := testPushMessage()
	message.Token = ""
	err := sender.Send(context.Background(), message)
	if err == nil {
		t.Fatal("want a validation error for an empty token")
	}
	if errors.Is(err, notifications.ErrInvalidDeviceToken) {
		t.Fatalf("Send error = %v, want transient classification", err)
	}
}
