//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/google/uuid"
)

func TestChatObservability_RedactsAuditPayloadsAndRecordsReconnectRecovery(t *testing.T) {
	var output bytes.Buffer
	audit := logging.NewAuditLogger(slog.New(slog.NewJSONHandler(&output, nil)))
	audit.LogChat(context.Background(), logging.AuditEvent{
		Action:      logging.ActionChatUpload,
		ActorUserID: "user-1",
		CircleID:    "circle-1",
		Metadata: map[string]any{
			"upload_id":  "upload-1",
			"object_key": "private/chat/object",
			"url":        "https://private.example/object",
			"body":       "private message body",
		},
	})

	metricsStore := new(metrics.ChatMetrics)
	metricsStore.RecordReconnect(metrics.ChatReconnectRecovered, time.Millisecond)
	summary := metricsStore.Summary()

	for _, leaked := range []string{"private/chat/object", "https://private.example/object", "private message body"} {
		if strings.Contains(output.String(), leaked) {
			t.Fatalf("chat audit leaked %q: %s", leaked, output.String())
		}
	}
	if !strings.Contains(output.String(), `"upload_id":"upload-1"`) {
		t.Fatalf("chat audit omitted safe upload attribution: %s", output.String())
	}
	if summary.Reconnects[metrics.ChatReconnectRecovered] != 1 {
		t.Fatalf("chat observability attribution = %+v", summary)
	}
}

func TestChatObservability_SearchFailureIsAttributedFromRealRepositoryPath(t *testing.T) {
	env := setupChatLifecycleEnv(t)
	ctx := context.Background()
	circle := env.createCircle(t, "creator", `{"name":"Chat Search Metrics","is_private":true}`)
	circleID := uuid.MustParse(circle.ID)
	creatorID := uuid.MustParse(env.userIDs["creator"])
	chatMetrics := new(metrics.ChatMetrics)
	service := chat.NewGroupService(chat.NewRepository(env.pool), env.circleRepo, chatMetrics, nil)

	if _, err := env.pool.Exec(ctx, "DROP TABLE messages CASCADE"); err != nil {
		t.Fatalf("break only the search repository query: %v", err)
	}
	if _, err := service.Search(ctx, creatorID, circleID, "search metric", nil, 50); err == nil {
		t.Fatal("search unexpectedly succeeded after its durable table was removed")
	}
	if got := chatMetrics.Summary().Searches[metrics.ChatSearchFailure]; got != 1 {
		t.Fatalf("search failure metric=%d, want 1 from the real repository failure", got)
	}
}

func TestChatObservability_SearchHydrationFailureIsAttributed(t *testing.T) {
	env := setupChatLifecycleEnv(t)
	ctx := context.Background()
	circle := env.createCircle(t, "creator", `{"name":"Chat Search Hydration Metrics","is_private":true}`)
	circleID := uuid.MustParse(circle.ID)
	creatorID := uuid.MustParse(env.userIDs["creator"])
	seedLifecycleMessage(t, env, creatorID, circleID, time.Now().Add(time.Minute), "hydration metric")
	chatMetrics := new(metrics.ChatMetrics)
	service := chat.NewGroupService(chat.NewRepository(env.pool), env.circleRepo, chatMetrics, nil)

	if _, err := env.pool.Exec(ctx, "DROP TABLE message_reads CASCADE"); err != nil {
		t.Fatalf("break only the search hydration query: %v", err)
	}
	if _, err := service.Search(ctx, creatorID, circleID, "hydration metric", nil, 50); err == nil {
		t.Fatal("search unexpectedly succeeded after its hydration table was removed")
	}
	if got := chatMetrics.Summary().Searches[metrics.ChatSearchFailure]; got != 1 {
		t.Fatalf("search hydration failure metric=%d, want 1", got)
	}
}

func TestChatObservability_RealUploadAndOutboxFailuresAreAttributed(t *testing.T) {
	t.Run("storage finalization failure", func(t *testing.T) {
		env := setupChatMediaEnv(t)
		ctx := context.Background()
		uploader := seedChatMediaUser(t, env.pool, "observability-uploader")
		circle := seedChatMediaCircle(t, env.pool, uploader)
		seedChatMediaMember(t, env.pool, circle, uploader, "teacher", time.Now().Add(-time.Hour))
		chatMetrics := new(metrics.ChatMetrics)
		service := chat.NewUploadService(
			chatMediaFailingUploadRepo{Repository: env.repo, failErr: errors.New("finalization unavailable")},
			env.members, env.store, env.cleaner, chatMetrics, nil,
		)
		if _, err := service.Stage(ctx, chat.StageUploadInput{
			UploaderID:       uploader,
			Target:           chat.UploadTarget{CircleID: &circle},
			MediaType:        chat.MessageTypeImage,
			Data:             chatMediaPNG(t),
			DeclaredFileName: "observability.png",
		}); err == nil {
			t.Fatal("stage unexpectedly succeeded after finalization failure")
		}
		if got := chatMetrics.Summary().Uploads[metrics.ChatUploadStorageFailure]; got != 1 {
			t.Fatalf("storage failure metric=%d, want 1 from the real upload path", got)
		}
	})

	t.Run("exhausted realtime backlog parks durably", func(t *testing.T) {
		env := setupChatLifecycleEnv(t)
		ctx := context.Background()
		circle := env.createCircle(t, "creator", `{"name":"Chat Outbox Metrics","is_private":true}`)
		creatorID := uuid.MustParse(env.userIDs["creator"])
		circleID := uuid.MustParse(circle.ID)
		repo := chat.NewRepository(env.pool)
		message, err := chat.NewGroupService(repo, env.circleRepo, new(metrics.ChatMetrics), nil).SendText(ctx, creatorID, circleID, "park this event", "observability-park")
		if err != nil {
			t.Fatalf("commit observability outbox event: %v", err)
		}
		chatMetrics := new(metrics.ChatMetrics)
		dispatcher := chat.NewOutboxDispatcher(chat.NewPGOutboxStore(repo), &recoveryProjector{err: errors.New("realtime unavailable")}, chatMetrics, nil, time.Now, func(delay time.Duration) time.Duration { return delay })
		for attempt := 0; attempt < 5; attempt++ {
			if err := dispatcher.DispatchDue(ctx, 1); err != nil {
				t.Fatalf("dispatch failing outbox attempt %d: %v", attempt+1, err)
			}
			if attempt < 4 {
				if _, err := env.pool.Exec(ctx, "UPDATE chat_event_outbox SET available_at = NOW() - INTERVAL '1 second' WHERE message_id = $1", message.ID); err != nil {
					t.Fatalf("make outbox retry due: %v", err)
				}
			}
		}
		if got := chatMetrics.Summary().Outbox[metrics.ChatOutboxParked]; got != 1 {
			t.Fatalf("parked metric=%d, want 1 after the durable retry ceiling", got)
		}
	})
}
