package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUploadServiceRejectsMalformedMedia(t *testing.T) {
	circle := uuid.New()
	for _, tc := range []struct {
		name     string
		kind     MessageType
		data     []byte
		duration int
	}{
		{"ogg header only", MessageTypeVoice, []byte("OggS"), 1},
		{"jpeg header only", MessageTypeImage, []byte{0xff, 0xd8, 0xff, 0xe0}, 0},
		{"pdf header only", MessageTypeFile, []byte("%PDF-1.7\n"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeUploadStore{}
			objects := &fakeObjectClient{}
			svc := newTestUploadService(repo, memberMembership(t, circle), objects, nil, time.Now())
			_, err := svc.Stage(context.Background(), groupStageInput(uuid.New(), circle, tc.kind, tc.data, tc.duration))
			if !errors.Is(err, ErrUnsupportedMIME) {
				t.Fatalf("malformed media accepted: %v", err)
			}
			if len(repo.inserts) != 0 || len(objects.puts) != 0 {
				t.Fatal("malformed media reached storage")
			}
		})
	}
}
