//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/google/uuid"
)

// TestChatReliability proves the database invariants that make retries safe:
// one sender/key pair converges to one message, conflicting payloads are
// rejected, and committed outbox rows remain recoverable after a crash.
func TestChatReliability(t *testing.T) {
	ctx := context.Background()
	admin := openPool(t, ctx)
	defer admin.Close()
	conn := acquireConn(t, admin, ctx)
	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	conn.Release()
	defer dropSchema(t, admin, ctx, schema)
	applyConn := acquireConn(t, admin, ctx)
	defer applyConn.Release()
	if _, err := applyConn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	applyRealTimeChatMigrations(t, applyConn, ctx)
	pool := openSchemaPool(t, ctx, schema)
	defer pool.Close()
	sender := seedLiveSessionUser(t, applyConn, ctx, "reliability-sender")
	circle := seedLiveSessionCircle(t, applyConn, ctx, sender)
	senderID, circleID := uuid.MustParse(sender), uuid.MustParse(circle)
	repo := chat.NewRepository(pool)
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1, $2, 'teacher')`, circleID, senderID); err != nil {
		t.Fatalf("seed reliability membership: %v", err)
	}

	t.Run("concurrent retry is one durable message", func(t *testing.T) {
		in := chat.MessageInput{SenderID: senderID, CircleID: &circleID, Type: chat.MessageTypeText, Content: "hello", IdempotencyKey: "retry-concurrent"}
		var wg sync.WaitGroup
		results := make(chan chat.Message, 2)
		errs := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var got chat.Message
				txErr := repo.WithTx(ctx, func(tx *chat.Tx) error {
					var inserted bool
					var err error
					got, inserted, err = tx.InsertMessage(ctx, in)
					if err == nil && inserted {
						err = tx.InsertOutboxEvent(ctx, got.ID, "chat.message", nil)
					}
					return err
				})
				if txErr != nil {
					errs <- txErr
				} else {
					results <- got
				}
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var ids []uuid.UUID
		for got := range results {
			ids = append(ids, got.ID)
		}
		if len(ids) != 2 || ids[0] != ids[1] {
			t.Fatalf("concurrent retry IDs=%v, want the same durable message", ids)
		}
		var messages, outbox int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE idempotency_key='retry-concurrent'").Scan(&messages); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM chat_event_outbox WHERE message_id=$1", ids[0]).Scan(&outbox); err != nil {
			t.Fatal(err)
		}
		if messages != 1 || outbox != 1 {
			t.Fatalf("messages=%d outbox=%d, want 1/1", messages, outbox)
		}
	})

	t.Run("different payload conflicts", func(t *testing.T) {
		in := chat.MessageInput{SenderID: senderID, CircleID: &circleID, Type: chat.MessageTypeText, Content: "first", IdempotencyKey: "retry-conflict"}
		if err := repo.WithTx(ctx, func(tx *chat.Tx) error { _, _, err := tx.InsertMessage(ctx, in); return err }); err != nil {
			t.Fatal(err)
		}
		conflict := in
		conflict.Content = "second"
		if err := repo.WithTx(ctx, func(tx *chat.Tx) error { _, _, err := tx.InsertMessage(ctx, conflict); return err }); err == nil {
			t.Fatal("different payload reused the idempotency key")
		}
	})

	t.Run("outbox crash recovery redelivers", func(t *testing.T) {
		var messageID uuid.UUID
		in := chat.MessageInput{SenderID: senderID, CircleID: &circleID, Type: chat.MessageTypeText, Content: "recover", IdempotencyKey: "retry-recovery"}
		if err := repo.WithTx(ctx, func(tx *chat.Tx) error {
			var err error
			var inserted bool
			var m chat.Message
			m, inserted, err = tx.InsertMessage(ctx, in)
			messageID = m.ID
			if err == nil && inserted {
				err = tx.InsertOutboxEvent(ctx, m.ID, "chat.message", nil)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		beforeCrash := chat.NewOutboxDispatcher(
			chat.NewPGOutboxStore(repo),
			&recoveryProjector{err: errors.New("connection lost")},
			&metrics.ChatMetrics{}, nil, time.Now,
			func(delay time.Duration) time.Duration { return delay },
		)
		if err := beforeCrash.DispatchDue(ctx, 10); err != nil {
			t.Fatal(err)
		}
		var attempts int
		if err := pool.QueryRow(ctx, "SELECT attempt_count FROM chat_event_outbox WHERE message_id=$1", messageID).Scan(&attempts); err != nil {
			t.Fatal(err)
		}
		if attempts != 1 {
			t.Fatalf("attempts=%d, want 1", attempts)
		}
		if _, err := pool.Exec(ctx, "UPDATE chat_event_outbox SET available_at=NOW()-INTERVAL '1 second' WHERE message_id=$1", messageID); err != nil {
			t.Fatal(err)
		}
		afterRestart := chat.NewOutboxDispatcher(
			chat.NewPGOutboxStore(repo),
			&recoveryProjector{},
			&metrics.ChatMetrics{}, nil, time.Now,
			func(delay time.Duration) time.Duration { return delay },
		)
		if err := afterRestart.DispatchDue(ctx, 10); err != nil {
			t.Fatal(err)
		}
		var delivered *time.Time
		if err := pool.QueryRow(ctx, "SELECT delivered_at FROM chat_event_outbox WHERE message_id=$1", messageID).Scan(&delivered); err != nil {
			t.Fatal(err)
		}
		if delivered == nil {
			t.Fatal("recovered outbox event was not delivered")
		}
	})

	t.Run("accepted retry is accounted per request", func(t *testing.T) {
		m := &metrics.ChatMetrics{}
		service := chat.NewGroupService(repo, rbac.NewRepository(pool), m, nil)
		first, err := service.SendText(ctx, senderID, circleID, "accounted retry", "retry-accounting")
		if err != nil {
			t.Fatalf("first accepted send: %v", err)
		}
		second, err := service.SendText(ctx, senderID, circleID, "accounted retry", "retry-accounting")
		if err != nil {
			t.Fatalf("accepted retry: %v", err)
		}
		if second.ID != first.ID {
			t.Fatalf("accepted retry message ID=%s, want committed ID %s", second.ID, first.ID)
		}
		if got := m.Summary().Outcomes[metrics.ChatOutcomeAccepted]; got != 2 {
			t.Fatalf("accepted retry count=%d, want 2 accepted operations", got)
		}
	})

	t.Run("postgres failure rejects durable send", func(t *testing.T) {
		failedPool := openSchemaPool(t, ctx, schema)
		failedPool.Close()
		failedRepo := chat.NewRepository(failedPool)
		err := failedRepo.WithTx(ctx, func(tx *chat.Tx) error {
			_, _, err := tx.InsertMessage(ctx, chat.MessageInput{SenderID: senderID, CircleID: &circleID, Type: chat.MessageTypeText, Content: "rejected", IdempotencyKey: "postgres-down"})
			return err
		})
		if err == nil {
			t.Fatal("send succeeded after PostgreSQL pool shutdown")
		}
	})
}

type recoveryProjector struct{ err error }

func (p *recoveryProjector) ProjectMessage(context.Context, chat.OutboxEvent, chat.Message) error {
	return p.err
}
