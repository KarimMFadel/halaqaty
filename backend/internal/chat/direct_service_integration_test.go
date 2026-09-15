//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

func TestDirectService_AllowedDirectionsAndHistoryRestoration(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-teacher")
	student := seedUser(t, repo, "dm-student")
	circle := seedCircle(t, repo, "Direct Service Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)
	service := NewDirectService(repo, nil)

	first, err := service.SendText(ctx, teacher, student, "feedback", "dm-key-teacher")
	if err != nil {
		t.Fatalf("teacher send: %v", err)
	}
	if _, err := service.SendText(ctx, student, teacher, "jazakallah", "dm-key-student"); err != nil {
		t.Fatalf("student send: %v", err)
	}
	history, err := service.History(ctx, teacher, student, nil, 50)
	if err != nil || len(history) != 2 {
		t.Fatalf("pair history = %d, err=%v; first=%s", len(history), err, first.ID)
	}

	if _, err := repo.pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1 AND user_id = $2`, circle, student); err != nil {
		t.Fatalf("remove student: %v", err)
	}
	if _, err := service.History(ctx, teacher, student, nil, 50); !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("history after last qualifying relationship = %v, want ErrDMNotEligible", err)
	}
	seedMember(t, repo, circle, student, "student", time.Now().UTC())
	restored, err := service.History(ctx, student, teacher, nil, 50)
	if err != nil || len(restored) != 2 {
		t.Fatalf("restored pair history = %d, err=%v", len(restored), err)
	}
}

func TestDirectService_DisallowedRolePairRejected(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	left := seedUser(t, repo, "dm-left")
	right := seedUser(t, repo, "dm-right")
	circle := seedCircle(t, repo, "Direct Denied Circle", left)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, left, "teacher", joined)
	seedMember(t, repo, circle, right, "supervisor", joined)
	service := NewDirectService(repo, nil)
	if _, err := service.SendText(ctx, left, right, "not allowed", "dm-denied"); !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("disallowed teacher-supervisor send = %v, want ErrDMNotEligible", err)
	}
}

func TestDirectService_RoleMatrixAndAnyQualifyingCircle(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	service := NewDirectService(repo, nil)

	tests := []struct {
		name       string
		leftRole   string
		rightRole  string
		shouldPass bool
	}{
		{name: "teacher student", leftRole: "teacher", rightRole: "student", shouldPass: true},
		{name: "student teacher", leftRole: "student", rightRole: "teacher", shouldPass: true},
		{name: "supervisor student", leftRole: "supervisor", rightRole: "student", shouldPass: true},
		{name: "student supervisor", leftRole: "student", rightRole: "supervisor", shouldPass: true},
		{name: "student student", leftRole: "student", rightRole: "student"},
		{name: "teacher teacher", leftRole: "teacher", rightRole: "teacher"},
		{name: "supervisor supervisor", leftRole: "supervisor", rightRole: "supervisor"},
		{name: "teacher supervisor", leftRole: "teacher", rightRole: "supervisor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := seedUser(t, repo, "dm-matrix-left-"+tt.name)
			right := seedUser(t, repo, "dm-matrix-right-"+tt.name)
			circle := seedCircle(t, repo, "Direct Matrix "+tt.name, left)
			joined := time.Now().UTC().Add(-time.Hour)
			seedMember(t, repo, circle, left, tt.leftRole, joined)
			seedMember(t, repo, circle, right, tt.rightRole, joined)

			_, err := service.SendText(ctx, left, right, "matrix", "dm-matrix-"+tt.name)
			if tt.shouldPass && err != nil {
				t.Fatalf("allowed pair send: %v", err)
			}
			if !tt.shouldPass && !errors.Is(err, ErrDMNotEligible) {
				t.Fatalf("denied pair send=%v, want ErrDMNotEligible", err)
			}
		})
	}

	teacher := seedUser(t, repo, "dm-multi-teacher")
	student := seedUser(t, repo, "dm-multi-student")
	first := seedCircle(t, repo, "Direct Multi First", teacher)
	second := seedCircle(t, repo, "Direct Multi Second", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	for _, circle := range []uuid.UUID{first, second} {
		seedMember(t, repo, circle, teacher, "teacher", joined)
		seedMember(t, repo, circle, student, "student", joined)
	}
	if _, err := service.SendText(ctx, teacher, student, "survives one circle", "dm-multi-1"); err != nil {
		t.Fatalf("multi-circle send: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1 AND user_id = $2`, first, student); err != nil {
		t.Fatalf("remove first qualifying circle: %v", err)
	}
	if history, err := service.History(ctx, student, teacher, nil, 50); err != nil || len(history) != 1 {
		t.Fatalf("history with one qualifying circle=%d, err=%v", len(history), err)
	}
	if _, err := repo.pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1 AND user_id = $2`, second, student); err != nil {
		t.Fatalf("remove last qualifying circle: %v", err)
	}
	if _, err := service.History(ctx, student, teacher, nil, 50); !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("history after last qualifying circle=%v, want ErrDMNotEligible", err)
	}
}

func TestDirectService_SendTextWaitsForRelationshipLock(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-lock-teacher")
	student := seedUser(t, repo, "dm-lock-student")
	circle := seedCircle(t, repo, "Direct Lock Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)

	blocker, err := repo.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker transaction: %v", err)
	}
	defer blocker.Rollback(ctx) //nolint:errcheck // cleanup after the assertion
	if _, err := blocker.Exec(ctx, `SELECT id FROM circles WHERE id = $1 FOR UPDATE`, circle); err != nil {
		t.Fatalf("lock circle: %v", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, err = NewDirectService(repo, nil).SendText(callCtx, teacher, student, "must wait", "dm-lock-key")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("send while relationship row is locked = %v, want context deadline", err)
	}
}

func TestDirectService_DeleteOwnMessageIsIdempotent(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-delete-teacher")
	student := seedUser(t, repo, "dm-delete-student")
	circle := seedCircle(t, repo, "Direct Delete Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)
	service := NewDirectService(repo, nil)

	message, err := service.SendText(ctx, teacher, student, "delete twice", "dm-delete-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := service.DeleteOwnMessage(ctx, teacher, student, message.ID); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := service.DeleteOwnMessage(ctx, teacher, student, message.ID); err != nil {
		t.Fatalf("idempotent delete retry: %v", err)
	}
}

func TestDirectService_DeleteTextWithMediaConfigured(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-text-delete-teacher")
	student := seedUser(t, repo, "dm-text-delete-student")
	circle := seedCircle(t, repo, "Direct Text Delete Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)
	uploads := NewUploadService(repo, nil, newTestMediaStore(&fakeObjectClient{}), nil, new(metrics.ChatMetrics), nil)
	service := NewDirectService(repo, uploads)

	message, err := service.SendText(ctx, teacher, student, "text only", "dm-text-delete-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := service.DeleteOwnMessage(ctx, teacher, student, message.ID); err != nil {
		t.Fatalf("delete text with media service configured: %v", err)
	}
}

func TestDirectService_DeleteMediaAppliesDeleteMarkerBeforeSoftDelete(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-media-delete-teacher")
	student := seedUser(t, repo, "dm-media-delete-student")
	circle := seedCircle(t, repo, "Direct Media Delete Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)
	fakeStore := &fakeObjectClient{}
	uploads := NewUploadService(repo, nil, newTestMediaStore(fakeStore), nil, new(metrics.ChatMetrics), nil)
	staged, err := repo.InsertUpload(ctx, Upload{
		UploaderID:            teacher,
		AuthorizationCircleID: circle,
		DMPeerID:              &student,
		ObjectKey:             "chat/direct-delete/object",
		MIMEType:              "image/png",
		OriginalFileName:      "image.png",
		SizeBytes:             10,
	})
	if err != nil {
		t.Fatalf("stage upload: %v", err)
	}
	var message Message
	if err := repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		message, _, err = tx.InsertMessage(ctx, MessageInput{
			SenderID:       teacher,
			DMRecipientID:  &student,
			Type:           MessageTypeImage,
			UploadID:       &staged.ID,
			IdempotencyKey: "dm-media-delete-send",
		})
		if err != nil {
			return err
		}
		_, err = tx.AttachUpload(ctx, staged.ID)
		return err
	}); err != nil {
		t.Fatalf("attach fixture: %v", err)
	}

	service := NewDirectService(repo, uploads)
	if err := service.DeleteOwnMessage(ctx, teacher, student, message.ID); err != nil {
		t.Fatalf("delete media: %v", err)
	}
	if len(fakeStore.removes) != 1 || fakeStore.removes[0].object != "chat/direct-delete/object" {
		t.Fatalf("delete markers = %+v, want one marker for attachment", fakeStore.removes)
	}
}
