//go:build integration

package chat

import (
	"context"
	"errors"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func TestUploadBudgetConcurrentFinalization(t *testing.T) {
	repo := newChatRepo(t)
	user := seedUser(t, repo, "budget")
	circle := seedCircle(t, repo, "budget", user)
	start := make(chan struct{})
	results := make(chan error, 20)
	var workers sync.WaitGroup
	for i := 0; i < 20; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := repo.InsertUploadWithinBudget(context.Background(), Upload{ID: uuid.New(), UploaderID: user, AuthorizationCircleID: circle, ObjectKey: "chat/" + uuid.NewString(), MIMEType: "image/png", OriginalFileName: "image.png", SizeBytes: 20}, time.Now().Add(-time.Hour))
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !errors.Is(err, ErrUploadRateExceeded) {
			t.Fatal(err)
		}
	}
	if accepted != MaxUploadsPerRollingHour {
		t.Fatalf("accepted %d concurrent uploads, want %d", accepted, MaxUploadsPerRollingHour)
	}
}

func TestMediaReplayRejectsChangedEnvelope(t *testing.T) {
	repo := newChatRepo(t)
	user := seedUser(t, repo, "replay")
	circle := seedCircle(t, repo, "original", user)
	other := seedCircle(t, repo, "other", user)
	upload, err := repo.InsertUpload(context.Background(), Upload{UploaderID: user, AuthorizationCircleID: circle, ObjectKey: "chat/" + uuid.NewString(), MIMEType: "image/png", OriginalFileName: "image.png", SizeBytes: 20})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewUploadService(repo, &stubMembershipReader{member: true, circle: rbac.Circle{ID: circle.String()}}, nil, nil, nil, nil)
	in := SendGroupMediaInput{SenderID: user, CircleID: circle, UploadID: upload.ID, MessageType: MessageTypeImage, IdempotencyKey: "replay-key"}
	original, err := svc.SendGroupMedia(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.SendGroupMedia(context.Background(), in)
	if err != nil || replay.ID != original.ID {
		t.Fatalf("unchanged replay: %v %v", replay, err)
	}
	for _, tc := range []SendGroupMediaInput{{SenderID: user, CircleID: other, UploadID: upload.ID, MessageType: MessageTypeImage, IdempotencyKey: in.IdempotencyKey}, {SenderID: user, CircleID: circle, UploadID: upload.ID, MessageType: MessageTypeVoice, IdempotencyKey: in.IdempotencyKey}} {
		if _, err := svc.SendGroupMedia(context.Background(), tc); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("changed replay: got %v", err)
		}
	}
}
