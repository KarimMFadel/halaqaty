package chat

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"
)

func TestMessageMediaProjection(t *testing.T) {
	circle, viewer, uploadID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	for _, kind := range []MessageType{MessageTypeVoice, MessageTypeImage, MessageTypeFile} {
		t.Run(string(kind), func(t *testing.T) {
			msg := Message{ID: uuid.New(), CircleID: &circle, SenderID: viewer, Type: kind, UploadID: &uploadID, State: MessageStateActive, SentAt: now}
			repo := &fakeUploadStore{found: true, foundMessage: msg, foundUpload: Upload{ID: uploadID, State: UploadStateAttached, ObjectKey: "chat/private", OriginalFileName: "../lesson\n.pdf", DurationSeconds: 2}}
			members := memberMembership(t, circle)
			svc := newTestUploadService(repo, members, &fakeObjectClient{}, nil, now)
			handler := NewGroupHandler(nil)
			handler.SetMediaService(svc)
			response, err := handler.projectMessage(context.Background(), viewer, msg)
			if err != nil {
				t.Fatal(err)
			}
			if response.MediaURL == "" || response.MediaURLExpiresAt == "" || response.FileName != "lesson.pdf" {
				t.Fatalf("missing media projection: %+v", response)
			}
			if kind == MessageTypeVoice && response.VoiceDurationSeconds != 2 {
				t.Fatalf("voice duration=%d", response.VoiceDurationSeconds)
			}
			if kind != MessageTypeVoice && response.VoiceDurationSeconds != 0 {
				t.Fatal("duration leaked onto nonvoice")
			}
			members.member = false
			if _, err := handler.projectMessage(context.Background(), viewer, msg); !errors.Is(err, ErrMessageNotVisible) {
				t.Fatalf("removed viewer projected media: %v", err)
			}
		})
	}
	text, err := NewGroupHandler(nil).projectMessage(context.Background(), viewer, Message{ID: uuid.New(), Type: MessageTypeText, Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "media_url") || strings.Contains(string(body), "file_name") || strings.Contains(string(body), "voice_duration") {
		t.Fatalf("text changed: %s", body)
	}
}
