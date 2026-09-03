package queue

import "unicode/utf8"

// maxNoteLength is the teacher-notes / progress-notes character bound
// (varchar(500)).
const maxNoteLength = 500

// maxIdempotencyKeyLength is the client Idempotency-Key character bound
// (varchar(128)).
const maxIdempotencyKeyLength = 128

// validationError builds a validation-class QueueError with a safe message
// that echoes no client input.
func validationError(message string) error {
	return &QueueError{Code: QueueErrorCodeValidation, Message: message}
}

// Validate reports whether s holds one of the five closed entry-status
// values; any other value is a validation error.
func (s EntryStatus) Validate() error {
	switch s {
	case EntryStatusWaiting, EntryStatusReciting, EntryStatusCompleted, EntryStatusSkipped, EntryStatusOptedOut:
		return nil
	default:
		return validationError("invalid entry status")
	}
}

// Validate reports whether l holds one of the three closed round-lifecycle
// values; any other value is a validation error.
func (l RoundLifecycle) Validate() error {
	switch l {
	case RoundLifecyclePrepared, RoundLifecycleActive, RoundLifecycleFinalized:
		return nil
	default:
		return validationError("invalid round lifecycle")
	}
}

// Validate reports whether t holds one of the four closed round-type values;
// any other value is a validation error.
func (t RoundType) Validate() error {
	switch t {
	case RoundTypeNewMemorization, RoundTypeRevision, RoundTypeOldRevision, RoundTypeTest:
		return nil
	default:
		return validationError("invalid round type")
	}
}

// Validate reports whether g holds one of the five closed ADR-013 grade
// values; any other value is a validation error.
func (g Grade) Validate() error {
	switch g {
	case GradeExcellent, GradeGood, GradeAcceptable, GradeNeedsReview, GradeRepeat:
		return nil
	default:
		return validationError("invalid grade")
	}
}

// Validate reports whether p holds one of the two closed population-policy
// values; any other value is a validation error.
func (p PopulationPolicy) Validate() error {
	switch p {
	case PopulationPolicyPresentAtActivation, PopulationPolicyAllActiveStudents:
		return nil
	default:
		return validationError("invalid population policy")
	}
}

// Validate reports whether p holds one of the two closed finalization-policy
// values; any other value is a validation error.
func (p FinalizationPolicy) Validate() error {
	switch p {
	case FinalizationPolicyMarkUnfinishedSkipped, FinalizationPolicyPreserveLastState:
		return nil
	default:
		return validationError("invalid finalization policy")
	}
}

// Validate reports whether p holds one of the two closed opt-out-policy
// values; any other value is a validation error.
func (p OptOutPolicy) Validate() error {
	switch p {
	case OptOutPolicyApprovalRequired, OptOutPolicyAutoApprove:
		return nil
	default:
		return validationError("invalid opt-out policy")
	}
}

// Validate reports whether v holds one of the three closed grade-visibility
// values; any other value is a validation error.
func (v GradeVisibility) Validate() error {
	switch v {
	case GradeVisibilityManagersAndStudent, GradeVisibilityManagersOnly, GradeVisibilityAllParticipants:
		return nil
	default:
		return validationError("invalid grade visibility")
	}
}

// Validate reports whether c holds one of the three closed grade-correction
// values; any other value is a validation error.
func (c GradeCorrection) Validate() error {
	switch c {
	case GradeCorrectionAuditedAnyTime, GradeCorrectionBeforeRoundFinalization, GradeCorrectionImmutable:
		return nil
	default:
		return validationError("invalid grade correction policy")
	}
}

// Validate reports whether s holds one of the three closed opt-out-request
// statuses; any other value is a validation error.
func (s OptOutRequestStatus) Validate() error {
	switch s {
	case OptOutRequestStatusPending, OptOutRequestStatusApproved, OptOutRequestStatusDeclined:
		return nil
	default:
		return validationError("invalid opt-out request status")
	}
}

// ValidateQuranRange validates a recitation range against injected bounds:
// surahID within 1..MaxSurahID, fromAyah at least 1, fromAyah at most
// toAyah, and toAyah at most surahAyahCount. It is pure — the caller
// supplies the authoritative per-surah ayah count from quran_surahs.
func ValidateQuranRange(surahID, fromAyah, toAyah, surahAyahCount int) error {
	switch {
	case surahID < 1 || surahID > MaxSurahID:
		return validationError("surah id outside 1..114")
	case fromAyah < 1:
		return validationError("from_ayah below 1")
	case fromAyah > toAyah:
		return validationError("from_ayah above to_ayah")
	case toAyah > surahAyahCount:
		return validationError("to_ayah above surah ayah count")
	}
	return nil
}

// ValidateNoteLength checks a teacher or progress note against the
// 500-character bound; characters are counted, not bytes, so Arabic notes
// are measured correctly.
func ValidateNoteLength(note string) error {
	if utf8.RuneCountInString(note) > maxNoteLength {
		return validationError("note above 500 characters")
	}
	return nil
}

// ValidateExpectedVersion checks an optimistic-concurrency expected version;
// only positive versions are valid.
func ValidateExpectedVersion(version int64) error {
	if version < 1 {
		return validationError("expected version below 1")
	}
	return nil
}

// ValidateIdempotencyKey checks a client Idempotency-Key against the
// 1..128-character bound.
func ValidateIdempotencyKey(key string) error {
	if n := utf8.RuneCountInString(key); n < 1 || n > maxIdempotencyKeyLength {
		return validationError("idempotency key outside 1..128 characters")
	}
	return nil
}
