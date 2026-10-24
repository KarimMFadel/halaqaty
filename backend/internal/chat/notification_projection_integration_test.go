//go:build integration

package chat

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
)

func TestGroupMessageNotificationTargetsOnlyEligibleCurrentMembers(t *testing.T) {
	ctx := context.Background()
	repo := newChatRepo(t)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000021_notification_system.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply F-008 migration: %v", err)
	}
	sender := seedUser(t, repo, "notice-sender")
	member := seedUser(t, repo, "notice-member")
	lateJoiner := seedUser(t, repo, "notice-late")
	circle := seedCircle(t, repo, "Notification Circle", sender)
	sentAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, sender, "teacher", sentAt.Add(-time.Hour))
	seedMember(t, repo, circle, member, "student", sentAt.Add(-time.Minute))
	seedMember(t, repo, circle, lateJoiner, "student", sentAt.Add(time.Minute))

	creator := notifications.NewRepository(repo.pool)
	projector := &RealtimeProjector{notifications: creator, listRecipients: repo.ListNotificationRecipients}
	message := Message{ID: uuid.New(), CircleID: &circle, SenderID: sender, SentAt: sentAt, Type: MessageTypeText, Content: "private content must not be copied"}
	eventID := uuid.New()
	event := OutboxEvent{EventID: eventID, MessageID: message.ID, EventType: "chat.message"}
	if err := projector.createGroupMessageNotifications(ctx, event, message); err != nil {
		t.Fatalf("project group message notices: %v", err)
	}
	if err := projector.createGroupMessageNotifications(ctx, event, message); err != nil {
		t.Fatalf("replay group message notices: %v", err)
	}
	page, err := creator.List(ctx, member.String(), nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].Type != notifications.EventGroupMessage {
		t.Fatalf("eligible member notices = %+v, error=%v", page.Items, err)
	}
	if page.Items[0].Body == message.Content || page.Items[0].SourceRef.ID != message.ID {
		t.Fatalf("notice exposed message content or wrong source: %+v", page.Items[0])
	}
	for _, id := range []uuid.UUID{sender, lateJoiner} {
		other, err := creator.List(ctx, id.String(), nil, 10)
		if err != nil || len(other.Items) != 0 {
			t.Fatalf("ineligible user %s notices = %+v, error=%v", id, other.Items, err)
		}
	}
}
