package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

const (
	// convergenceTarget is the maximum time F-003 has to finalize all rounds
	// of an ended session, measured from when the end fact is observed.
	convergenceTarget = 10 * time.Second
	// convergenceRetryInterval is the base delay between idempotent retries
	// when a finalization attempt fails transiently.
	convergenceRetryInterval = 500 * time.Millisecond
)

// Convergence drives session-end queue finalization independently of the F-005
// session lifecycle. It finalizes the active round and every never-activated
// prepared round so that ended sessions leave no actionable queue state.
type Convergence struct {
	repo    *Repository
	metrics *metrics.QueueMetrics
	logger  *slog.Logger
	now     func() time.Time
}

// NewConvergence constructs a queue convergence driver.
func NewConvergence(repo *Repository, metrics *metrics.QueueMetrics, logger *slog.Logger) *Convergence {
	return &Convergence{repo: repo, metrics: metrics, logger: logger, now: time.Now}
}

// OnSessionEnded is the F-005 observer callback. It returns immediately after
// starting a bounded background finalization; the F-005 session-end result is
// never altered or blocked by queue cleanup.
func (c *Convergence) OnSessionEnded(ctx context.Context, sessionID string) {
	go c.finalizeWithTimeout(sessionID)
}

func (c *Convergence) finalizeWithTimeout(sessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), convergenceTarget)
	defer cancel()
	if err := c.FinalizeSession(ctx, sessionID); err != nil && c.logger != nil {
		c.logger.Error("queue session-end convergence did not complete within target",
			"session_id", sessionID, "error", err)
	}
}

// FinalizeSession idempotently finalizes every active and prepared round for
// the ended session, retrying transient failures until the context expires.
func (c *Convergence) FinalizeSession(ctx context.Context, sessionID string) error {
	deadline := c.deadline(ctx)

	for {
		rounds, err := c.repo.SessionRoundsToFinalize(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("list rounds to finalize: %w", err)
		}
		if len(rounds) == 0 {
			return nil
		}

		remaining := c.finalizeRounds(ctx, sessionID, rounds)
		if len(remaining) == 0 {
			return nil
		}

		if c.now().After(deadline) || ctx.Err() != nil {
			if c.metrics != nil {
				c.metrics.RecordSessionEndFinalizationLag(convergenceTarget)
			}
			return fmt.Errorf("convergence deadline reached with %d unfinalized rounds", len(remaining))
		}

		if err := c.waitRetry(ctx, deadline); err != nil {
			return err
		}
	}
}

func (c *Convergence) deadline(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return c.now().Add(convergenceTarget)
}

func (c *Convergence) waitRetry(ctx context.Context, deadline time.Time) error {
	delay := convergenceRetryInterval
	if left := time.Until(deadline); left < delay {
		delay = left
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// finalizeRounds finalizes the provided rounds in order. It returns the subset
// that could not be finalized (empty on full success).
func (c *Convergence) finalizeRounds(ctx context.Context, sessionID string, rounds []Round) []Round {
	var remaining []Round
	for _, round := range rounds {
		if err := c.finalizeRound(ctx, sessionID, round); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Round was finalized concurrently; retry loop will observe it gone.
				continue
			}
			remaining = append(remaining, round)
			if c.logger != nil {
				c.logger.Warn("queue round finalization failed, will retry", "session_id", sessionID, "round_id", round.ID, "error", err)
			}
		}
	}
	return remaining
}

func (c *Convergence) finalizeRound(ctx context.Context, sessionID string, round Round) error {
	return c.repo.withSessionRoundLock(ctx, sessionID, func(runTx queueTxRunner) error {
		return runTx(ctx, func(tx *Tx) error {
			policy, err := tx.LockSessionPolicy(ctx, sessionID)
			if err != nil {
				return err
			}
			current, err := tx.LockRound(ctx, round.ID)
			if err != nil {
				return err
			}
			if current.Lifecycle == RoundLifecycleFinalized {
				return nil
			}
			finalized, err := tx.FinalizeRound(ctx, current.ID, current.Version, policy.Policy.Finalization, "")
			if err != nil {
				return err
			}
			return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: sessionID, RoundID: current.ID,
				EventType: queueEventRoundFinalized, RoundVersion: finalized.Version, EventMetadata: json.RawMessage(`{"reason":"session_ended"}`)})
		})
	})
}

// Reconcile scans for ended sessions that still have active or prepared rounds
// and finalizes them. It is intended for startup recovery (CHK033).
func (c *Convergence) Reconcile(ctx context.Context) error {
	sessionIDs, err := c.repo.SessionsNeedingConvergence(ctx)
	if err != nil {
		return fmt.Errorf("find sessions needing convergence: %w", err)
	}
	for _, sessionID := range sessionIDs {
		if err := c.FinalizeSession(ctx, sessionID); err != nil {
			if c.logger != nil {
				c.logger.Error("queue convergence reconcile failed", "session_id", sessionID, "error", err)
			}
		}
	}
	return nil
}
