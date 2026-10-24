package notifications

import (
	"testing"
	"time"
)

// TestDefaultDispatchJitterStaysWithinBounds verifies the default ±10% jitter
// keeps every retry delay inside [0.9x, 1.1x] of the exponential base across
// the five-attempt ladder while still varying the schedule.
func TestDefaultDispatchJitterStaysWithinBounds(t *testing.T) {
	const samples = 2000
	for attempt := 1; attempt <= MaxPushAttempts; attempt++ {
		base := time.Duration(1) << (attempt - 1) * time.Second
		lower := base - base/10
		upper := base + base/10
		seen := make(map[time.Duration]struct{}, samples)
		for i := 0; i < samples; i++ {
			got := defaultDispatchJitter(base)
			if got < lower || got > upper {
				t.Fatalf("attempt %d: jitter(%s)=%s, want within [%s, %s]", attempt, base, got, lower, upper)
			}
			seen[got] = struct{}{}
		}
		if len(seen) < 2 {
			t.Fatalf("attempt %d: jitter produced %d distinct values, want variation", attempt, len(seen))
		}
	}
	if got := defaultDispatchJitter(0); got != 0 {
		t.Fatalf("jitter(0)=%s, want 0", got)
	}
}
