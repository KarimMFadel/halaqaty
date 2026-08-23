package sessions

import (
	"context"
	"time"
)

// defaultQueueObserverTimeout bounds one queue-observer callback dispatch.
// F-005 lifecycle transactions must never wait on F-003 side effects (plan
// D1); five seconds is generous for a fact notification and short enough that
// a stuck queue cannot stall session lifecycle callers.
const defaultQueueObserverTimeout = 5 * time.Second

// QueueObserver is the narrow, optional hook through which the sessions
// domain reports committed lifecycle and presence facts to the F-003
// recitation queue (plan D1). Implementations receive only committed facts
// after the corresponding F-005 transaction has succeeded; they must never be
// on the critical path of that transaction (see BoundedQueueObserver).
//
// Parameter types are deliberately primitive strings: no queue-domain type
// leaks into the sessions package, and no session-media material ever flows
// through this boundary.
type QueueObserver interface {
	// OnSessionStarted reports that sessionID transitioned to active.
	OnSessionStarted(ctx context.Context, sessionID string) error
	// OnParticipantJoined reports that userID joined sessionID.
	OnParticipantJoined(ctx context.Context, sessionID, userID string) error
	// OnSessionEnded reports that sessionID transitioned to ended.
	OnSessionEnded(ctx context.Context, sessionID string) error
}

// BoundedQueueObserver wraps a QueueObserver so a slow, failing, or panicking
// queue callback can never block or roll back an F-005 lifecycle transaction:
// every dispatch runs under a per-call timeout, panics are recovered, and
// callback errors are swallowed. The wrapped observer receives the facts; the
// caller gets no error surface to accidentally propagate.
//
// A callback that ignores its context is abandoned at the timeout, not
// killed; its goroutine ends whenever the callback returns. Callback errors
// and panics are dropped silently: the sessions package has no logging
// facade, and committed F-005 truth is authoritative by design (plan D10/D11
// reconciliation repairs anything the queue missed).
type BoundedQueueObserver struct {
	next    QueueObserver
	timeout time.Duration
}

// NewBoundedQueueObserver wraps next with the per-callback bound
// perCallTimeout; a non-positive value selects defaultQueueObserverTimeout.
// A nil next yields a no-op observer, so optional wiring stays safe.
func NewBoundedQueueObserver(next QueueObserver, perCallTimeout time.Duration) *BoundedQueueObserver {
	if perCallTimeout <= 0 {
		perCallTimeout = defaultQueueObserverTimeout
	}
	return &BoundedQueueObserver{next: next, timeout: perCallTimeout}
}

// OnSessionStarted reports the committed session-start fact; see the
// BoundedQueueObserver contract.
func (b *BoundedQueueObserver) OnSessionStarted(ctx context.Context, sessionID string) error {
	b.dispatch(ctx, func(callCtx context.Context) error {
		return b.next.OnSessionStarted(callCtx, sessionID)
	})
	return nil
}

// OnParticipantJoined reports the committed participant-join fact; see the
// BoundedQueueObserver contract.
func (b *BoundedQueueObserver) OnParticipantJoined(ctx context.Context, sessionID, userID string) error {
	b.dispatch(ctx, func(callCtx context.Context) error {
		return b.next.OnParticipantJoined(callCtx, sessionID, userID)
	})
	return nil
}

// OnSessionEnded reports the committed session-end fact; see the
// BoundedQueueObserver contract.
func (b *BoundedQueueObserver) OnSessionEnded(ctx context.Context, sessionID string) error {
	b.dispatch(ctx, func(callCtx context.Context) error {
		return b.next.OnSessionEnded(callCtx, sessionID)
	})
	return nil
}

// dispatch runs one callback under the per-call bound and swallows every
// outcome: the caller returns at the latest when the bound expires, and
// neither errors nor panics cross back.
func (b *BoundedQueueObserver) dispatch(ctx context.Context, call func(context.Context) error) {
	if b.next == nil {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if recover() != nil {
				done <- nil
			}
		}()
		done <- call(callCtx)
	}()
	select {
	case <-done:
		// Swallowed by design: F-005 commits stay authoritative.
	case <-callCtx.Done():
		// Bound exceeded; the callback goroutine is abandoned to finish (or
		// hang) on its own. Nothing is propagated to the F-005 caller.
	}
}
