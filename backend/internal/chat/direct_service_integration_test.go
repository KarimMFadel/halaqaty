//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"
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
