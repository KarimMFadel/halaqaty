//go:build integration

package notifications_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

// seedClaimableIntent inserts one due pending_attempt row directly, so claim
// races do not depend on the registration wake-up path.
func seedClaimableIntent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, sourceEventID string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, push_expires_at, delivery_state, next_attempt_at
) VALUES ($1, 'group_message', $2, 'message', $3, 'New message', 'Open the app to view it',
          NOW() + INTERVAL '1 hour', 'pending_attempt', NOW() - INTERVAL '1 minute')
RETURNING id`, userID, sourceEventID, uuid.New()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// T015 concurrency: N workers claiming from M due intents never receive the
// same intent twice (FOR UPDATE SKIP LOCKED + lease) and collectively claim
// every due intent exactly once.
func TestConcurrentClaimDueIntentsAreDisjointAndComplete(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete claim-race integration user: %v", err)
		}
	})
	repo := notifications.NewRepository(pool)

	const intentCount = 12
	seeded := make(map[uuid.UUID]struct{}, intentCount)
	for i := 0; i < intentCount; i++ {
		seeded[seedClaimableIntent(t, ctx, pool, userID, fmt.Sprintf("race-claim-%d", i))] = struct{}{}
	}

	const workers = 4
	const batch = 2
	start := make(chan struct{})
	claimed := make(chan uuid.UUID, intentCount*workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				intents, err := repo.ClaimDueIntents(ctx, time.Now().UTC(), batch)
				if err != nil {
					t.Errorf("claim batch: %v", err)
					return
				}
				if len(intents) == 0 {
					return
				}
				for _, intent := range intents {
					claimed <- intent.ID
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(claimed)

	seen := make(map[uuid.UUID]int, intentCount)
	for id := range claimed {
		seen[id]++
	}
	if len(seen) != intentCount {
		t.Fatalf("distinct claimed intents=%d, want %d", len(seen), intentCount)
	}
	for id := range seeded {
		if seen[id] != 1 {
			t.Fatalf("intent %s claimed %d times, want exactly once", id, seen[id])
		}
	}
	// Everything due was claimed and leased: a further claim returns nothing.
	intents, err := repo.ClaimDueIntents(ctx, time.Now().UTC(), intentCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 0 {
		t.Fatalf("post-drain claim returned %d intents, want none (all leased)", len(intents))
	}
}

// T015 concurrency: an expiry sweep racing a claim leaves the row in exactly
// one guarded terminal outcome — the loser observes the winner's state and no
// transition resurrects the intent into claimability.
func TestClaimVersusExpireRaceLeavesSingleTerminalOutcome(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete expire-race integration user: %v", err)
		}
	})
	repo := notifications.NewRepository(pool)

	const rounds = 25
	for round := 0; round < rounds; round++ {
		intentID := seedClaimableIntent(t, ctx, pool, userID, fmt.Sprintf("race-expire-%d", round))
		claimNow := time.Now().UTC()
		// The deadline sits between the two racing clocks: the claim still
		// sees the row as due, the sweep sees it as expired.
		expireNow := claimNow.Add(2 * time.Second)
		if _, err := pool.Exec(ctx,
			"UPDATE notifications SET push_expires_at = $2 WHERE id = $1", intentID, claimNow.Add(time.Second)); err != nil {
			t.Fatal(err)
		}

		start := make(chan struct{})
		var claimed int
		var expired int64
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			intents, err := repo.ClaimDueIntents(ctx, claimNow, 10)
			if err != nil {
				t.Errorf("racing claim: %v", err)
				return
			}
			claimed = len(intents)
		}()
		go func() {
			defer wg.Done()
			<-start
			swept, err := repo.ExpireDueIntents(ctx, expireNow, 10)
			if err != nil {
				t.Errorf("racing expiry sweep: %v", err)
				return
			}
			expired = swept
		}()
		close(start)
		wg.Wait()

		// The deadline is past for the sweep clock, so expiry always applies
		// (whether the claim leased the row first is the race). The intent may
		// have been claimed zero or one time — never more.
		// Assumes package-level serialization: no t.Parallel and one shared DB.
		if expired != 1 {
			t.Fatalf("round %d: swept=%d, want the expired row swept exactly once", round, expired)
		}
		if claimed > 1 {
			t.Fatalf("round %d: claimed=%d, want at most one claim", round, claimed)
		}
		if state := deliveryStateOf(t, ctx, pool, intentID); state != "expired" {
			t.Fatalf("round %d: final state=%q, want terminal expired", round, state)
		}
		// No resurrection: the terminal row is never claimable again, even
		// after any lease could have lapsed.
		intents, err := repo.ClaimDueIntents(ctx, expireNow.Add(2*time.Minute), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(intents) != 0 {
			t.Fatalf("round %d: terminal intent reclaimed %v, want none", round, intents)
		}
		// Clean the round's row so the next round's sweep count stays exact.
		if _, err := pool.Exec(ctx, "DELETE FROM notifications WHERE id = $1", intentID); err != nil {
			t.Fatal(err)
		}
	}
}
