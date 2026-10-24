package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// DefaultCleanupInterval is the periodic retention cadence used when Run
	// receives a non-positive interval.
	DefaultCleanupInterval = time.Hour
	// DefaultCleanupLimit bounds one cleanup batch per statement at MVP scale.
	// Repeated cycles drain any backlog, so a large backlog never turns one
	// cycle into an unbounded delete.
	DefaultCleanupLimit = 500
)

// CleanupStore is the persistence seam for retention cleanup;
// *Repository implements it on PostgreSQL.
type CleanupStore interface {
	// DeleteTerminalAttemptState removes a bounded batch of attempt rows
	// belonging to notifications in a terminal delivery state.
	DeleteTerminalAttemptState(ctx context.Context, limit int) (int64, error)
	// DeleteExpiredHistory removes a bounded batch of notification rows past
	// the 30-day window (cascading to attempts), preserving pending required
	// deletion intents until they reach a terminal delivery state.
	DeleteExpiredHistory(ctx context.Context, limit int) (int64, error)
}

// Cleaner enforces the ADR-027 §6 retention policy: history stays visible for
// the rolling 30-day window, expired rows and terminal attempt state are
// deleted in bounded batches, and pending required manager-deletion intents
// survive past the window until terminal handling. Cleanup is independent of
// the dispatcher and never blocks dispatch.
type Cleaner struct {
	store   CleanupStore
	metrics *NotificationMetrics
}

// NewCleaner constructs a retention cleaner. A nil metrics receiver is safe.
func NewCleaner(store CleanupStore, notificationMetrics *NotificationMetrics) *Cleaner {
	return &Cleaner{store: store, metrics: notificationMetrics}
}

// CleanOnce deletes one bounded batch of terminal attempt state, then one
// bounded batch of expired history. The first store failure aborts the cycle
// with a wrapped error; the next cycle retries, so expired rows are never
// lost by a transient failure. Deleted counts are recorded as metrics and
// logged without any user content or tokens.
func (c *Cleaner) CleanOnce(ctx context.Context, limit int) error {
	if c.store == nil {
		return errors.New("notifications cleaner requires a store")
	}
	if limit < 1 {
		limit = DefaultCleanupLimit
	}
	attempts, err := c.store.DeleteTerminalAttemptState(ctx, limit)
	if err != nil {
		return fmt.Errorf("delete terminal notification attempt state: %w", err)
	}
	history, err := c.store.DeleteExpiredHistory(ctx, limit)
	if err != nil {
		return fmt.Errorf("delete expired notification history: %w", err)
	}
	c.metrics.RecordCleanupDeleted(attempts, history)
	if attempts > 0 || history > 0 {
		slog.InfoContext(ctx, "notification retention cleanup",
			"attempt_rows_deleted", attempts, "history_rows_deleted", history)
	}
	return nil
}

// Run drives periodic retention cleanup until ctx is canceled. Per-cycle
// failures are tolerated and logged (no tokens or user content); expected
// cancellation during shutdown is not. The shape mirrors Dispatcher.Run so
// both share one lifecycle.
func (c *Cleaner) Run(ctx context.Context, limit int, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultCleanupInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := c.CleanOnce(ctx, limit); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "notification retention cleanup cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Repository implements CleanupStore on PostgreSQL.
var _ CleanupStore = (*Repository)(nil)

// DeleteTerminalAttemptState removes a bounded batch of attempt rows whose
// notification already settled into a terminal delivery state and returns how
// many rows were deleted.
func (r *Repository) DeleteTerminalAttemptState(ctx context.Context, limit int) (int64, error) {
	tag, err := r.pool.Exec(ctx, deleteTerminalAttemptStateQuery, limit)
	if err != nil {
		return 0, fmt.Errorf("delete terminal notification attempt state: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredHistory removes a bounded batch of notification rows past the
// 30-day window — the FK cascade removes their attempt rows — while keeping
// preference_override rows that are still in a pending delivery state. It
// returns how many notification rows were deleted.
func (r *Repository) DeleteExpiredHistory(ctx context.Context, limit int) (int64, error) {
	tag, err := r.pool.Exec(ctx, deleteExpiredHistoryQuery, limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired notification history: %w", err)
	}
	return tag.RowsAffected(), nil
}
