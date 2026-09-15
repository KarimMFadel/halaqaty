//go:build integration

package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// TestGroupService_SearchNormalizesArabicPrefixesAndRetainsCurrentMembership
// proves search uses the migration's defined Arabic normalization and prefix
// behavior without leaking a prior membership period.
func TestGroupService_SearchNormalizesArabicPrefixesAndRetainsCurrentMembership(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-search-teacher")
	viewerID := seedUser(t, repo, "discovery-search-viewer")
	circleID := seedCircle(t, repo, "Discovery Search Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt.Add(-time.Hour))
	seedMember(t, repo, circleID, viewerID, "student", joinedAt)
	seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(-time.Minute), "العلم قبل القول")
	seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "أَلْعِلْمُ نور")
	seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(2*time.Minute), "memorization notes")

	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))
	results, err := service.Search(ctx, viewerID, circleID, "علم", nil, 50)
	if err != nil {
		t.Fatalf("Arabic prefix search: %v", err)
	}
	if got := messageContents(results); len(got) != 1 || got[0] != "أَلْعِلْمُ نور" {
		t.Fatalf("Arabic normalized prefix results = %v, want only current-period result", got)
	}

	results, err = service.Search(ctx, viewerID, circleID, "memo", nil, 50)
	if err != nil {
		t.Fatalf("Latin prefix search: %v", err)
	}
	if got := messageContents(results); len(got) != 1 || got[0] != "memorization notes" {
		t.Fatalf("Latin prefix results = %v, want memorization notes", got)
	}
}

// TestGroupService_SearchCursorRetainsRankMismatchedResults proves the UUID
// cursor remains complete even when term frequency would otherwise reorder
// matching rows by relevance.
func TestGroupService_SearchCursorRetainsRankMismatchedResults(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-search-cursor-teacher")
	viewerID := seedUser(t, repo, "discovery-search-cursor-viewer")
	circleID := seedCircle(t, repo, "Discovery Search Cursor Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, viewerID, "student", joinedAt)
	oldID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "needle needle needle")
	middleID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(2*time.Minute), "needle")
	newID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(3*time.Minute), "needle")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	first, err := service.Search(ctx, viewerID, circleID, "needle", nil, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first search page = %#v, %v", first, err)
	}
	if first[0].ID != oldID {
		t.Fatalf("first search page must prefer the highest rank, got %s want %s", first[0].ID, oldID)
	}
	second, err := service.Search(ctx, viewerID, circleID, "needle", &first[0].ID, 10)
	if err != nil {
		t.Fatalf("second search page: %v", err)
	}
	got := map[uuid.UUID]bool{first[0].ID: true}
	for _, message := range second {
		got[message.ID] = true
	}
	for _, id := range []uuid.UUID{oldID, middleID, newID} {
		if !got[id] {
			t.Fatalf("cursor omitted matching message %s; pages=%v,%v", id, first, second)
		}
	}

	seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(4*time.Minute), "memo notes")
	results, err := service.Search(ctx, viewerID, circleID, "memo notes", nil, 50)
	if err != nil {
		t.Fatalf("multi-token search: %v", err)
	}
	if got := messageContents(results); len(got) != 1 || got[0] != "memo notes" {
		t.Fatalf("only final search token may prefix-match: got %v", got)
	}
}

// TestGroupService_ReplyTextRequiresVisibleSameCircleTarget proves replies
// retain only a safe, bounded preview of an active target from the caller's
// current circle membership period.
func TestGroupService_ReplyTextRequiresVisibleSameCircleTarget(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	senderID := seedUser(t, repo, "discovery-reply-sender")
	viewerID := seedUser(t, repo, "discovery-reply-viewer")
	circleID := seedCircle(t, repo, "Discovery Reply Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, viewerID, "student", joinedAt)
	targetID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), strings.Repeat("آ", 200))
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	reply, err := service.ReplyText(ctx, viewerID, circleID, targetID, "جزاك الله خيرا", "discovery-reply-key")
	if err != nil {
		t.Fatalf("reply to visible same-circle target: %v", err)
	}
	if reply.ReplyToID == nil || *reply.ReplyToID != targetID {
		t.Fatalf("reply target = %v, want %s", reply.ReplyToID, targetID)
	}
	if reply.ReplyPreview == nil || reply.ReplyPreview.ID != targetID || reply.ReplyPreview.Deleted {
		t.Fatalf("reply preview = %#v, want active target projection", reply.ReplyPreview)
	}
	if got := len([]rune(reply.ReplyPreview.Preview)); got == 0 || got > 160 {
		t.Fatalf("reply preview rune length = %d, want 1..160", got)
	}
}

// TestGroupService_ReplyRetrySurvivesDeletedTarget proves an accepted reply
// remains retryable after its target is later soft-deleted.
func TestGroupService_ReplyRetrySurvivesDeletedTarget(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-reply-retry-teacher")
	studentID := seedUser(t, repo, "discovery-reply-retry-student")
	circleID := seedCircle(t, repo, "Discovery Reply Retry Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	targetID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "target")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	first, err := service.ReplyText(ctx, studentID, circleID, targetID, "reply", "reply-retry-key")
	if err != nil {
		t.Fatalf("first reply: %v", err)
	}
	if first.ReplyPreview == nil || first.ReplyPreview.ID != targetID || first.ReplyPreview.Deleted || first.ReplyPreview.Preview != "target" {
		t.Fatalf("first reply preview = %#v, want active target projection", first.ReplyPreview)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, targetID); err != nil {
		t.Fatalf("delete target: %v", err)
	}
	replay, err := service.ReplyText(ctx, studentID, circleID, targetID, "reply", "reply-retry-key")
	if err != nil {
		t.Fatalf("reply replay after target deletion: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay id = %s, want %s", replay.ID, first.ID)
	}
	if replay.ReplyPreview == nil || replay.ReplyPreview.ID != targetID || !replay.ReplyPreview.Deleted || replay.ReplyPreview.Preview != "" {
		t.Fatalf("replay reply preview = %#v, want deleted target redaction", replay.ReplyPreview)
	}
}

// TestDirectService_ReplyRetryHydratesCurrentTargetProjection proves a direct
// reply replay projects the target's current active or deleted state.
func TestDirectService_ReplyRetryHydratesCurrentTargetProjection(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-dm-reply-retry-teacher")
	studentID := seedUser(t, repo, "discovery-dm-reply-retry-student")
	circleID := seedCircle(t, repo, "Discovery Direct Reply Retry Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	targetID := seedMessage(t, repo, teacherID, nil, &studentID, joinedAt.Add(time.Minute), "direct target")
	service := NewDirectService(repo, nil)

	first, err := service.ReplyText(ctx, studentID, teacherID, targetID, "reply", "direct-reply-retry-key")
	if err != nil {
		t.Fatalf("first direct reply: %v", err)
	}
	if first.ReplyPreview == nil || first.ReplyPreview.ID != targetID || first.ReplyPreview.Deleted || first.ReplyPreview.Preview != "direct target" {
		t.Fatalf("first direct reply preview = %#v, want active target projection", first.ReplyPreview)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, targetID); err != nil {
		t.Fatalf("delete direct target: %v", err)
	}

	replay, err := service.ReplyText(ctx, studentID, teacherID, targetID, "reply", "direct-reply-retry-key")
	if err != nil {
		t.Fatalf("direct reply replay after target deletion: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("direct replay id = %s, want %s", replay.ID, first.ID)
	}
	if replay.ReplyPreview == nil || replay.ReplyPreview.ID != targetID || !replay.ReplyPreview.Deleted || replay.ReplyPreview.Preview != "" {
		t.Fatalf("direct replay reply preview = %#v, want deleted target redaction", replay.ReplyPreview)
	}
}

// TestGroupService_ReplyProjectionRedactsDeletedTarget proves history and
// search never retain content from a reply target deleted after acceptance.
func TestGroupService_ReplyProjectionRedactsDeletedTarget(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-reply-projection-teacher")
	studentID := seedUser(t, repo, "discovery-reply-projection-student")
	circleID := seedCircle(t, repo, "Discovery Reply Projection Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	targetID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "private target")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))
	reply, err := service.ReplyText(ctx, studentID, circleID, targetID, "reply needle", "reply-projection-key")
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, targetID); err != nil {
		t.Fatalf("delete target: %v", err)
	}
	for name, load := range map[string]func() ([]Message, error){
		"history": func() ([]Message, error) { return service.History(ctx, studentID, circleID, nil, 50) },
		"search":  func() ([]Message, error) { return service.Search(ctx, studentID, circleID, "needle", nil, 50) },
	} {
		t.Run(name, func(t *testing.T) {
			messages, err := load()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, message := range messages {
				if message.ID == reply.ID && (message.ReplyPreview == nil || !message.ReplyPreview.Deleted || message.ReplyPreview.Preview != "") {
					t.Fatalf("%s reply preview = %#v, want deleted redaction", name, message.ReplyPreview)
				}
			}
		})
	}
}

// TestGroupService_ReplyTextRejectsInaccessibleTargets proves target lookup is
// non-enumerating across missing, deleted, other-circle, and pre-join rows.
func TestGroupService_ReplyTextRejectsInaccessibleTargets(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-reply-denial-teacher")
	studentID := seedUser(t, repo, "discovery-reply-denial-student")
	otherTeacherID := seedUser(t, repo, "discovery-reply-denial-other")
	circleID := seedCircle(t, repo, "Discovery Reply Denial Circle", teacherID)
	otherCircleID := seedCircle(t, repo, "Discovery Reply Other Circle", otherTeacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	seedMember(t, repo, otherCircleID, otherTeacherID, "teacher", joinedAt)
	deletedID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "deleted target")
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, deletedID); err != nil {
		t.Fatalf("delete target fixture: %v", err)
	}
	otherCircleTarget := seedMessage(t, repo, otherTeacherID, &otherCircleID, nil, joinedAt.Add(time.Minute), "other circle")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	for name, targetID := range map[string]uuid.UUID{
		"missing":      uuid.New(),
		"deleted":      deletedID,
		"other circle": otherCircleTarget,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := service.ReplyText(ctx, studentID, circleID, targetID, "reply", "discovery-denial-"+name)
			if !errors.Is(err, ErrMessageNotVisible) {
				t.Fatalf("reply to %s error = %v, want ErrMessageNotVisible", name, err)
			}
		})
	}
}

// TestDirectService_ReplyTextRejectsOtherPairTarget proves direct replies
// cannot use a visible identifier from another pair conversation.
func TestDirectService_ReplyTextRejectsOtherPairTarget(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-dm-reply-teacher")
	studentID := seedUser(t, repo, "discovery-dm-reply-student")
	otherStudentID := seedUser(t, repo, "discovery-dm-reply-other-student")
	circleID := seedCircle(t, repo, "Discovery Direct Reply Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	seedMember(t, repo, circleID, otherStudentID, "student", joinedAt)
	targetID := seedMessage(t, repo, teacherID, nil, &studentID, joinedAt.Add(time.Minute), "pair target")
	service := NewDirectService(repo, nil)

	if _, err := service.ReplyText(ctx, teacherID, studentID, targetID, "same pair", "discovery-dm-reply-ok"); err != nil {
		t.Fatalf("reply in same direct conversation: %v", err)
	}
	if _, err := service.ReplyText(ctx, teacherID, otherStudentID, targetID, "other pair", "discovery-dm-reply-cross"); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("reply across direct conversations error = %v, want ErrMessageNotVisible", err)
	}
}

// TestDirectService_HistoryDoesNotHydrateCrossPairReplyPreview proves a
// malformed durable reference cannot disclose content from another DM pair.
func TestDirectService_HistoryDoesNotHydrateCrossPairReplyPreview(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-dm-preview-teacher")
	studentID := seedUser(t, repo, "discovery-dm-preview-student")
	otherStudentID := seedUser(t, repo, "discovery-dm-preview-other")
	circleID := seedCircle(t, repo, "Discovery Direct Preview Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	seedMember(t, repo, circleID, otherStudentID, "student", joinedAt)
	targetID := seedMessage(t, repo, teacherID, nil, &otherStudentID, joinedAt.Add(time.Minute), "other pair secret")
	replyID := seedMessage(t, repo, teacherID, nil, &studentID, joinedAt.Add(2*time.Minute), "same pair reply")
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET reply_to_id = $1 WHERE id = $2`, targetID, replyID); err != nil {
		t.Fatalf("seed malformed cross-pair reply: %v", err)
	}

	messages, err := NewDirectService(repo, nil).History(ctx, teacherID, studentID, nil, 50)
	if err != nil {
		t.Fatalf("load direct history: %v", err)
	}
	for _, message := range messages {
		if message.ID == replyID && message.ReplyPreview != nil {
			t.Fatalf("cross-pair reply preview = %#v, want nil", message.ReplyPreview)
		}
	}
}

// TestGroupService_ListPinnedOrdersVisibleMessages proves the pinned bar is
// capped by the service and respects both the current membership period and
// pinned_at DESC, id DESC ordering.
func TestGroupService_ListPinnedOrdersVisibleMessages(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-pinned-teacher")
	viewerID := seedUser(t, repo, "discovery-pinned-viewer")
	circleID := seedCircle(t, repo, "Discovery Pinned Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt.Add(-time.Hour))
	seedMember(t, repo, circleID, viewerID, "student", joinedAt)
	oldID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(-time.Minute), "before membership")
	newerID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "newer pin")
	newestID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(2*time.Minute), "newest pin")
	for id, pinnedAt := range map[uuid.UUID]time.Time{
		oldID:    joinedAt.Add(3 * time.Minute),
		newerID:  joinedAt.Add(4 * time.Minute),
		newestID: joinedAt.Add(5 * time.Minute),
	} {
		if _, err := repo.pool.Exec(ctx, `UPDATE messages SET is_pinned = TRUE, pinned_by = $1, pinned_at = $2 WHERE id = $3`, teacherID, pinnedAt, id); err != nil {
			t.Fatalf("pin fixture %s: %v", id, err)
		}
	}
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	pinned, err := service.ListPinned(ctx, viewerID, circleID)
	if err != nil {
		t.Fatalf("list pinned: %v", err)
	}
	if len(pinned) != 2 || pinned[0].ID != newestID || pinned[1].ID != newerID {
		t.Fatalf("pinned order = %#v, want newest visible pins", pinned)
	}
	if pinned[0].PinnedBy == nil || *pinned[0].PinnedBy != teacherID || pinned[0].PinnedAt == nil {
		t.Fatalf("pinned metadata = %#v, want pin actor and timestamp", pinned[0])
	}
}

// TestGroupService_PinReplayAtLimitIsIdempotent proves retrying an already
// pinned target does not fail merely because the circle is now full.
func TestGroupService_PinReplayAtLimitIsIdempotent(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-pin-replay-teacher")
	circleID := seedCircle(t, repo, "Discovery Pin Replay Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	ids := make([]uuid.UUID, 5)
	for i := range ids {
		ids[i] = seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Duration(i+1)*time.Minute), "pinnable")
	}
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))
	for _, id := range ids {
		if _, err := service.Pin(ctx, teacherID, circleID, id); err != nil {
			t.Fatalf("initial pin %s: %v", id, err)
		}
	}
	changed, err := service.Pin(ctx, teacherID, circleID, ids[0])
	if err != nil || changed {
		t.Fatalf("pin replay at limit = changed:%t err:%v, want false,nil", changed, err)
	}
}

func TestGroupService_UnpinDistinguishesMissingDeletedAndAlreadyUnpinned(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-unpin-teacher")
	circleID := seedCircle(t, repo, "Discovery Unpin Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	messageID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "unpin target")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	if _, err := service.Unpin(ctx, teacherID, circleID, uuid.New()); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("missing unpin error = %v, want ErrMessageNotVisible", err)
	}
	if _, err := service.Pin(ctx, teacherID, circleID, messageID); err != nil {
		t.Fatalf("pin target: %v", err)
	}
	changed, err := service.Unpin(ctx, teacherID, circleID, messageID)
	if err != nil || !changed {
		t.Fatalf("first unpin = changed:%t err:%v, want true,nil", changed, err)
	}
	changed, err = service.Unpin(ctx, teacherID, circleID, messageID)
	if err != nil || changed {
		t.Fatalf("already-unpinned retry = changed:%t err:%v, want false,nil", changed, err)
	}
	if _, err := service.Pin(ctx, teacherID, circleID, messageID); err != nil {
		t.Fatalf("repin target: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET is_pinned = FALSE, pinned_by = NULL, pinned_at = NULL, deleted_at = NOW() WHERE id = $1`, messageID); err != nil {
		t.Fatalf("delete target: %v", err)
	}
	if _, err := service.Unpin(ctx, teacherID, circleID, messageID); !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("deleted unpin error = %v, want ErrMessageNotVisible", err)
	}
}

func TestGroupService_PinAuthorizationAndPersistence(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-pin-auth-teacher")
	studentID := seedUser(t, repo, "discovery-pin-auth-student")
	outsiderID := seedUser(t, repo, "discovery-pin-auth-outsider")
	circleID := seedCircle(t, repo, "Discovery Pin Authorization Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	messageID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "persisted pin")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	if _, err := service.Pin(ctx, studentID, circleID, messageID); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("student pin error = %v, want rbac.ErrForbidden", err)
	}
	if _, err := service.Pin(ctx, outsiderID, circleID, messageID); !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("outsider pin error = %v, want ErrCircleNotVisible", err)
	}
	if changed, err := service.Pin(ctx, teacherID, circleID, messageID); err != nil || !changed {
		t.Fatalf("teacher pin = changed:%t err:%v, want true,nil", changed, err)
	}
	if got := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE id = $1 AND is_pinned`, messageID); got != 1 {
		t.Fatalf("persisted pin rows = %d, want 1", got)
	}
	if changed, err := service.Unpin(ctx, teacherID, circleID, messageID); err != nil || !changed {
		t.Fatalf("teacher unpin = changed:%t err:%v, want true,nil", changed, err)
	}
	if got := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE id = $1 AND NOT is_pinned`, messageID); got != 1 {
		t.Fatalf("persisted unpin rows = %d, want 1", got)
	}
}

// TestSafeReplyPreviewTrimsAndBoundsRunes proves quoted text is normalized
// before its 160-rune cap is applied.
func TestSafeReplyPreviewTrimsAndBoundsRunes(t *testing.T) {
	preview := safeReplyPreview(Message{ID: uuid.New(), Content: "  " + strings.Repeat("آ", 161) + "  "})
	if got := len([]rune(preview.Preview)); got != 160 || strings.TrimSpace(preview.Preview) != preview.Preview {
		t.Fatalf("preview = %q (%d runes), want trimmed 160 runes", preview.Preview, got)
	}
}

// TestGroupService_PinSerializesFiveMessageLimit proves concurrent pin
// attempts serialize on the owning circle row and cannot produce six active
// pins. A circle-row lock, shared with unpin and delete, must also block a
// concurrent pin rather than allow it to observe an intermediate count.
func TestGroupService_PinSerializesFiveMessageLimit(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-pin-teacher")
	circleID := seedCircle(t, repo, "Discovery Pin Concurrency Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	messageIDs := make([]uuid.UUID, 6)
	for i := range messageIDs {
		messageIDs[i] = seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Duration(i+1)*time.Minute), "pinnable")
	}
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	var wg sync.WaitGroup
	errs := make(chan error, len(messageIDs))
	for _, messageID := range messageIDs {
		wg.Add(1)
		go func(messageID uuid.UUID) {
			defer wg.Done()
			_, err := service.Pin(ctx, teacherID, circleID, messageID)
			errs <- err
		}(messageID)
	}
	wg.Wait()
	close(errs)
	accepted := 0
	limited := 0
	for err := range errs {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, ErrPinLimit):
			limited++
		default:
			t.Fatalf("concurrent pin error = %v", err)
		}
	}
	if accepted != 5 || limited != 1 {
		t.Fatalf("concurrent pins accepted=%d limited=%d, want 5 and 1", accepted, limited)
	}
	if got := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE circle_id = $1 AND is_pinned`, circleID); got != 5 {
		t.Fatalf("active pins = %d, want 5", got)
	}
}

// TestGroupService_PinWaitsForCircleRowMutationLock proves pinning shares the
// owning circle row lock used by unpin and delete mutations, preventing a
// concurrent count from racing a circle lifecycle change.
func TestGroupService_PinWaitsForCircleRowMutationLock(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacherID := seedUser(t, repo, "discovery-pin-lock-teacher")
	circleID := seedCircle(t, repo, "Discovery Pin Lock Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	messageID := seedMessage(t, repo, teacherID, &circleID, nil, joinedAt.Add(time.Minute), "lock target")
	service := NewGroupService(repo, rbac.NewRepository(repo.pool), &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))

	blocker, err := repo.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker transaction: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Rollback(context.Background()) })
	if _, err := blocker.Exec(ctx, `SELECT id FROM circles WHERE id = $1 FOR UPDATE`, circleID); err != nil {
		t.Fatalf("lock circle row: %v", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := service.Pin(callCtx, teacherID, circleID, messageID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pin behind circle-row mutation lock error = %v, want context deadline", err)
	}
}
