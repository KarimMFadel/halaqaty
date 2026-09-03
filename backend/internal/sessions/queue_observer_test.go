package sessions

import (
	"context"
	"errors"
	"testing"
	"time"
)

var _ QueueObserver = (*BoundedQueueObserver)(nil)

// recordingObserver captures every committed fact the wrapper dispatches.
// Tests read its slices only after the wrapper method has returned, which
// happens after the callback completed, so no synchronization is needed.
type recordingObserver struct {
	started []string
	joined  [][2]string
	ended   []string

	returnErr error
	block     chan struct{} // when non-nil, the callback ignores ctx and blocks
	panicOn   bool
}

func (o *recordingObserver) OnSessionStarted(_ context.Context, sessionID string) error {
	o.enter()
	o.started = append(o.started, sessionID)
	return o.returnErr
}

func (o *recordingObserver) OnParticipantJoined(_ context.Context, sessionID, userID string) error {
	o.enter()
	o.joined = append(o.joined, [2]string{sessionID, userID})
	return o.returnErr
}

func (o *recordingObserver) OnSessionEnded(_ context.Context, sessionID string) error {
	o.enter()
	o.ended = append(o.ended, sessionID)
	return o.returnErr
}

// enter models the worst-case callback: one that ignores its context. When
// panicOn is set it panics instead.
func (o *recordingObserver) enter() {
	if o.panicOn {
		panic("queue callback exploded")
	}
	if o.block != nil {
		<-o.block
	}
}

func TestBoundedQueueObserverDispatchesAllFacts(t *testing.T) {
	obs := &recordingObserver{}
	bounded := NewBoundedQueueObserver(obs, time.Second)

	_ = bounded.OnSessionStarted(context.Background(), "session-1")
	_ = bounded.OnParticipantJoined(context.Background(), "session-1", "user-1")
	_ = bounded.OnSessionEnded(context.Background(), "session-1")

	if len(obs.started) != 1 || obs.started[0] != "session-1" {
		t.Fatalf("started facts = %v, want [session-1]", obs.started)
	}
	if len(obs.joined) != 1 || obs.joined[0] != [2]string{"session-1", "user-1"} {
		t.Fatalf("joined facts = %v, want [[session-1 user-1]]", obs.joined)
	}
	if len(obs.ended) != 1 || obs.ended[0] != "session-1" {
		t.Fatalf("ended facts = %v, want [session-1]", obs.ended)
	}
}

func TestBoundedQueueObserverBoundsHangingCallback(t *testing.T) {
	unblock := make(chan struct{})
	defer close(unblock)
	obs := &recordingObserver{block: unblock}
	bounded := NewBoundedQueueObserver(obs, 50*time.Millisecond)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = bounded.OnParticipantJoined(context.Background(), "session-1", "user-1")
	}()
	select {
	case <-done:
		// The wrapper returned even though the callback still hangs.
	case <-time.After(2 * time.Second):
		t.Fatal("a hanging queue callback must never block the F-005 caller beyond the bound")
	}
}

func TestBoundedQueueObserverRecoversPanickingCallback(t *testing.T) {
	obs := &recordingObserver{panicOn: true}
	bounded := NewBoundedQueueObserver(obs, time.Second)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Must not panic the caller.
		_ = bounded.OnSessionEnded(context.Background(), "session-1")
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a panicking queue callback must not hang or crash the F-005 caller")
	}
}

func TestBoundedQueueObserverSwallowsCallbackError(t *testing.T) {
	obs := &recordingObserver{returnErr: errors.New("queue side effect failed")}
	bounded := NewBoundedQueueObserver(obs, time.Second)

	// The wrapper does not propagate callback errors; the contract under test
	// is that a failing callback does not become a panic or a hang.
	_ = bounded.OnSessionStarted(context.Background(), "session-1")
	_ = bounded.OnParticipantJoined(context.Background(), "session-1", "user-1")
	_ = bounded.OnSessionEnded(context.Background(), "session-1")

	if len(obs.started)+len(obs.joined)+len(obs.ended) != 3 {
		t.Fatal("failing callbacks must still be dispatched")
	}
}

func TestNewBoundedQueueObserverDefaultsTimeout(t *testing.T) {
	bounded := NewBoundedQueueObserver(&recordingObserver{}, 0)
	if bounded.timeout != defaultQueueObserverTimeout {
		t.Fatalf("timeout = %v, want the %v default", bounded.timeout, defaultQueueObserverTimeout)
	}
}
