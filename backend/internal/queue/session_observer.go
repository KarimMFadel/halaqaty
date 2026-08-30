package queue

import "context"

// SessionObserver applies committed F-005 lifecycle facts to the display-only
// queue. It intentionally has no media dependency or audio operation.
type SessionObserver struct{ rounds *RoundService }

// NewSessionObserver constructs the F-003 observer consumed by sessions.
func NewSessionObserver(rounds *RoundService) *SessionObserver {
	return &SessionObserver{rounds: rounds}
}

// OnSessionStarted restores the active-round invariant after F-005 commits.
func (o *SessionObserver) OnSessionStarted(ctx context.Context, sessionID string) error {
	return o.rounds.ActivateIfNeeded(ctx, sessionID)
}

// OnParticipantJoined appends one durable waiting entry for a late joiner.
func (o *SessionObserver) OnParticipantJoined(ctx context.Context, sessionID, userID string) error {
	return o.rounds.AppendLateJoiner(ctx, sessionID, userID)
}

// OnSessionEnded is reserved for the later queue finalization task.
func (o *SessionObserver) OnSessionEnded(context.Context, string) error { return nil }
