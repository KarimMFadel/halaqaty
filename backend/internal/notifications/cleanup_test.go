package notifications_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

type fakeCleanupStore struct {
	attemptsLimit, historyLimit int
	attempts, history           int64
	attemptErr, historyErr      error
	calls                       int
}

func (s *fakeCleanupStore) DeleteTerminalAttemptState(_ context.Context, limit int) (int64, error) {
	s.calls++
	s.attemptsLimit = limit
	return s.attempts, s.attemptErr
}

func (s *fakeCleanupStore) DeleteExpiredHistory(_ context.Context, limit int) (int64, error) {
	s.historyLimit = limit
	return s.history, s.historyErr
}

// T013: one bounded cleanup cycle deletes terminal attempt state first, then
// expired history, applies the default bound on a non-positive limit, and
// records the deleted counts as a metric signal.
func TestCleanerCleanOnceBoundsBatchesAndRecordsMetrics(t *testing.T) {
	store := &fakeCleanupStore{attempts: 4, history: 7}
	metrics := &notifications.NotificationMetrics{}
	cleaner := notifications.NewCleaner(store, metrics)

	if err := cleaner.CleanOnce(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if store.attemptsLimit != notifications.DefaultCleanupLimit || store.historyLimit != notifications.DefaultCleanupLimit {
		t.Fatalf("limits=(%d, %d), want the default bound %d for both statements",
			store.attemptsLimit, store.historyLimit, notifications.DefaultCleanupLimit)
	}
	summary := metrics.Summary()
	if summary.CleanupAttemptsDeleted != 4 || summary.CleanupHistoryDeleted != 7 {
		t.Fatalf("cleanup metrics=(%d, %d), want (4, 7)",
			summary.CleanupAttemptsDeleted, summary.CleanupHistoryDeleted)
	}
}

// T013: a store failure aborts the cycle with a wrapped error; the next cycle
// retries, so expired rows are never lost by a transient failure.
func TestCleanerCleanOnceWrapsStoreFailures(t *testing.T) {
	store := &fakeCleanupStore{attemptErr: errors.New("db down")}
	cleaner := notifications.NewCleaner(store, nil)
	if err := cleaner.CleanOnce(context.Background(), 10); err == nil {
		t.Fatal("want wrapped attempt-state failure")
	}

	store = &fakeCleanupStore{historyErr: errors.New("db down")}
	cleaner = notifications.NewCleaner(store, nil)
	if err := cleaner.CleanOnce(context.Background(), 10); err == nil {
		t.Fatal("want wrapped history failure")
	}
}

// T013: Run is context-driven like Dispatcher.Run: it cleans on schedule and
// stops cleanly on cancellation without blocking dispatch.
func TestCleanerRunStopsOnCancellation(t *testing.T) {
	store := &fakeCleanupStore{}
	cleaner := notifications.NewCleaner(store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cleaner.Run(ctx, 10, time.Hour) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil on cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
	if store.calls == 0 {
		t.Fatal("Run should execute an immediate startup cycle before waiting")
	}
}
