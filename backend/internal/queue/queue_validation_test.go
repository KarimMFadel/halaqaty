package queue

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// assertValidationError fails the test when err is not a validation-class
// QueueError; it is the shared assertion for every invalid-case table row.
func assertValidationError(t *testing.T, err error) {
	t.Helper()
	var qe *QueueError
	if !errors.As(err, &qe) {
		t.Fatalf("got error %v (%T), want *QueueError", err, err)
	}
	if qe.Code != QueueErrorCodeValidation {
		t.Fatalf("got QueueError code %q, want %q", qe.Code, QueueErrorCodeValidation)
	}
}

// TestClosedEnums checks every closed enum: each data-model value is accepted
// and anything else is rejected as a validation-class QueueError.
func TestClosedEnums(t *testing.T) {
	tests := []struct {
		name     string
		validate func(string) error
		valid    []string
		invalid  []string
	}{
		{
			name:     "entry status",
			validate: func(s string) error { return EntryStatus(s).Validate() },
			valid:    []string{"waiting", "reciting", "completed", "skipped", "opted_out"},
			invalid:  []string{"", "Waiting", "waiting ", "done", "paused", "optedout"},
		},
		{
			name:     "round lifecycle",
			validate: func(s string) error { return RoundLifecycle(s).Validate() },
			valid:    []string{"prepared", "active", "finalized"},
			invalid:  []string{"", "Preparing", "closed", "archived", "cancelled"},
		},
		{
			name:     "round type",
			validate: func(s string) error { return RoundType(s).Validate() },
			valid:    []string{"new_memorization", "revision", "old_revision", "test"},
			invalid:  []string{"", "new memorization", "New_Memorization", "quiz", "talaqi"},
		},
		{
			name:     "grade",
			validate: func(s string) error { return Grade(s).Validate() },
			valid:    []string{"excellent", "good", "acceptable", "needs_review", "repeat"},
			invalid:  []string{"", "Excellent", "needs review", "fail", "pass", "meh"},
		},
		{
			name:     "population policy",
			validate: func(s string) error { return PopulationPolicy(s).Validate() },
			valid:    []string{"present_at_activation", "all_active_students"},
			invalid:  []string{"", "present", "all_students", "everyone", "present_at_start"},
		},
		{
			name:     "finalization policy",
			validate: func(s string) error { return FinalizationPolicy(s).Validate() },
			valid:    []string{"mark_unfinished_skipped", "preserve_last_state"},
			invalid:  []string{"", "mark_skipped", "preserve", "skip_unfinished"},
		},
		{
			name:     "opt-out policy",
			validate: func(s string) error { return OptOutPolicy(s).Validate() },
			valid:    []string{"approval_required", "auto_approve"},
			invalid:  []string{"", "approval", "auto", "always_allow", "manual"},
		},
		{
			name:     "grade visibility",
			validate: func(s string) error { return GradeVisibility(s).Validate() },
			valid:    []string{"managers_and_student", "managers_only", "all_participants"},
			invalid:  []string{"", "managers", "everyone", "student_only", "public"},
		},
		{
			name:     "grade correction",
			validate: func(s string) error { return GradeCorrection(s).Validate() },
			valid:    []string{"audited_any_time", "before_round_finalization", "immutable"},
			invalid:  []string{"", "any_time", "frozen", "before_finalization", "locked"},
		},
		{
			name:     "opt-out request status",
			validate: func(s string) error { return OptOutRequestStatus(s).Validate() },
			valid:    []string{"pending", "approved", "declined"},
			invalid:  []string{"", "Pending", "rejected", "cancelled", "withdrawn"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range tt.valid {
				if err := tt.validate(v); err != nil {
					t.Errorf("valid value %q rejected: %v", v, err)
				}
			}
			for _, v := range tt.invalid {
				err := tt.validate(v)
				if err == nil {
					t.Errorf("invalid value %q accepted", v)
					continue
				}
				assertValidationError(t, err)
			}
		})
	}
}

// TestCanTransitionEntry walks the full 5×5 entry-status matrix: exactly the
// six legal data-model transitions return true; every other pair — including
// all terminal→x and self transitions — returns false.
func TestCanTransitionEntry(t *testing.T) {
	legal := map[EntryStatus][]EntryStatus{
		EntryStatusWaiting:  {EntryStatusReciting, EntryStatusSkipped, EntryStatusOptedOut},
		EntryStatusReciting: {EntryStatusCompleted, EntryStatusSkipped, EntryStatusOptedOut},
	}
	all := []EntryStatus{
		EntryStatusWaiting,
		EntryStatusReciting,
		EntryStatusCompleted,
		EntryStatusSkipped,
		EntryStatusOptedOut,
	}
	for _, from := range all {
		for _, to := range all {
			want := slices.Contains(legal[from], to)
			if got := CanTransitionEntry(from, to); got != want {
				t.Errorf("CanTransitionEntry(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

// TestValidateQuranRange covers the injected-bounds range rules: surah
// 1..114, from_ayah ≥ 1, from_ayah ≤ to_ayah, to_ayah ≤ injected ayah count.
func TestValidateQuranRange(t *testing.T) {
	tests := []struct {
		name                             string
		surahID, fromAyah, toAyah, count int
		wantErr                          bool
	}{
		{name: "full surah Al-Fatiha", surahID: 1, fromAyah: 1, toAyah: 7, count: 7},
		{name: "single ayah", surahID: 36, fromAyah: 1, toAyah: 1, count: 83},
		{name: "last surah full", surahID: 114, fromAyah: 1, toAyah: 6, count: 6},
		{name: "mid-range band", surahID: 2, fromAyah: 50, toAyah: 60, count: 286},
		{name: "to equals count", surahID: 112, fromAyah: 1, toAyah: 4, count: 4},
		{name: "surah id zero", surahID: 0, fromAyah: 1, toAyah: 1, count: 7, wantErr: true},
		{name: "surah id above 114", surahID: 115, fromAyah: 1, toAyah: 1, count: 7, wantErr: true},
		{name: "negative surah id", surahID: -3, fromAyah: 1, toAyah: 1, count: 7, wantErr: true},
		{name: "from below one", surahID: 1, fromAyah: 0, toAyah: 3, count: 7, wantErr: true},
		{name: "negative from", surahID: 1, fromAyah: -2, toAyah: 3, count: 7, wantErr: true},
		{name: "from above to", surahID: 1, fromAyah: 5, toAyah: 4, count: 7, wantErr: true},
		{name: "to above count", surahID: 1, fromAyah: 1, toAyah: 8, count: 7, wantErr: true},
		{name: "single ayah above count", surahID: 18, fromAyah: 2, toAyah: 2, count: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuranRange(tt.surahID, tt.fromAyah, tt.toAyah, tt.count)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("valid range rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid range accepted")
			}
			assertValidationError(t, err)
		})
	}
}

// TestValidateNoteLength covers the ≤ 500-character note bound; characters
// are counted, not bytes, so 500 Arabic characters (1500 bytes) stay valid.
func TestValidateNoteLength(t *testing.T) {
	tests := []struct {
		name    string
		note    string
		wantErr bool
	}{
		{name: "empty note", note: ""},
		{name: "500 ascii characters", note: strings.Repeat("a", 500)},
		{name: "500 arabic characters", note: strings.Repeat("م", 500)},
		{name: "501 ascii characters", note: strings.Repeat("a", 501), wantErr: true},
		{name: "501 arabic characters", note: strings.Repeat("م", 501), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoteLength(tt.note)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("valid note rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid note accepted")
			}
			assertValidationError(t, err)
		})
	}
}

// TestValidateExpectedVersion covers the optimistic-concurrency guard: only
// positive expected versions are valid; zero and negative are rejected.
func TestValidateExpectedVersion(t *testing.T) {
	tests := []struct {
		name    string
		version int64
		wantErr bool
	}{
		{name: "first version", version: 1},
		{name: "later version", version: 42},
		{name: "zero version", version: 0, wantErr: true},
		{name: "negative version", version: -1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpectedVersion(tt.version)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("valid version rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid version accepted")
			}
			assertValidationError(t, err)
		})
	}
}

// TestValidateIdempotencyKey covers the client Idempotency-Key bound of
// 1..128 characters: empty and 129-character keys are rejected.
func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "single character", key: "k"},
		{name: "128 characters", key: strings.Repeat("k", 128)},
		{name: "empty key", key: "", wantErr: true},
		{name: "129 characters", key: strings.Repeat("k", 129), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdempotencyKey(tt.key)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("valid key rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid key accepted")
			}
			assertValidationError(t, err)
		})
	}
}
