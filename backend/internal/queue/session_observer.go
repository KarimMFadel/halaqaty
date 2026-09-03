package queue

import "context"

// SessionObserver applies committed F-005 lifecycle facts to the display-only
// queue. It intentionally has no media dependency or audio operation.
type SessionObserver struct {
	rounds      *RoundService
	convergence *Convergence
}

// NewSessionObserver constructs the F-003 observer consumed by sessions.
func NewSessionObserver(rounds *RoundService, convergence *Convergence) *SessionObserver {
	return &SessionObserver{rounds: rounds, convergence: convergence}
}

// OnSessionStarted restores the active-round invariant after F-005 commits.
func (o *SessionObserver) OnSessionStarted(ctx context.Context, sessionID string) error {
	return o.rounds.ActivateIfNeeded(ctx, sessionID)
}

// OnParticipantJoined appends one durable waiting entry for a late joiner.
func (o *SessionObserver) OnParticipantJoined(ctx context.Context, sessionID, userID string) error {
	return o.rounds.AppendLateJoiner(ctx, sessionID, userID)
}

// OnSessionEnded hands the end fact to the convergence driver so F-005 returns
// immediately while queue finalization retries idempotently in the background.
func (o *SessionObserver) OnSessionEnded(ctx context.Context, sessionID string) error {
	if o.convergence != nil {
		o.convergence.OnSessionEnded(ctx, sessionID)
	}
	return nil
}
