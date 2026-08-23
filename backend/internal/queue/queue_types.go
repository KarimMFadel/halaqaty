// Package queue owns the F-003 recitation-queue domain: rounds and entries,
// pre-activation ordering, the five queue policy dimensions, opt-out
// requests, idempotent command receipts, the transactional outbox, and
// memorization progress records. It is provider-neutral: no media types,
// room references, or credentials appear in this package.
package queue

import (
	"encoding/json"
	"time"
)

// EntryStatus is the closed state of a round's queue entry.
type EntryStatus string

// Entry states; completed, skipped, and opted_out are terminal for the round.
const (
	// EntryStatusWaiting is a materialized entry not yet reciting.
	EntryStatusWaiting EntryStatus = "waiting"
	// EntryStatusReciting is the single active reciter of the round.
	EntryStatusReciting EntryStatus = "reciting"
	// EntryStatusCompleted is a finished recitation turn.
	EntryStatusCompleted EntryStatus = "completed"
	// EntryStatusSkipped is a manager-skipped entry.
	EntryStatusSkipped EntryStatus = "skipped"
	// EntryStatusOptedOut is an approved or auto-approved opt-out.
	EntryStatusOptedOut EntryStatus = "opted_out"
)

// RoundLifecycle is the closed lifecycle of a round, separate from entry
// states; finalized is terminal and a finalized never-activated round is
// permanently inert.
type RoundLifecycle string

// Round lifecycle values.
const (
	// RoundLifecyclePrepared is a stacked round awaiting activation.
	RoundLifecyclePrepared RoundLifecycle = "prepared"
	// RoundLifecycleActive is the single active round of a live session.
	RoundLifecycleActive RoundLifecycle = "active"
	// RoundLifecycleFinalized is a terminal round; history, never reopened.
	RoundLifecycleFinalized RoundLifecycle = "finalized"
)

// RoundType is the closed classification of a recitation round.
type RoundType string

// Round types.
const (
	// RoundTypeNewMemorization is a first-time memorization turn.
	RoundTypeNewMemorization RoundType = "new_memorization"
	// RoundTypeRevision is a recent-material revision turn.
	RoundTypeRevision RoundType = "revision"
	// RoundTypeOldRevision is an older-material revision turn.
	RoundTypeOldRevision RoundType = "old_revision"
	// RoundTypeTest is an assessment turn; excluded from Quran-map derivation.
	RoundTypeTest RoundType = "test"
)

// Grade is the closed ADR-013 recitation grade.
type Grade string

// Canonical grade values.
const (
	// GradeExcellent is the top recitation grade.
	GradeExcellent Grade = "excellent"
	// GradeGood is a strong recitation grade.
	GradeGood Grade = "good"
	// GradeAcceptable is a passing recitation grade.
	GradeAcceptable Grade = "acceptable"
	// GradeNeedsReview signals a recitation needing another listen.
	GradeNeedsReview Grade = "needs_review"
	// GradeRepeat requires the same range again next turn.
	GradeRepeat Grade = "repeat"
)

// PopulationPolicy selects which students a round activation materializes.
type PopulationPolicy string

// Population policies.
const (
	// PopulationPolicyPresentAtActivation materializes present students.
	PopulationPolicyPresentAtActivation PopulationPolicy = "present_at_activation"
	// PopulationPolicyAllActiveStudents materializes all active members.
	PopulationPolicyAllActiveStudents PopulationPolicy = "all_active_students"
)

// FinalizationPolicy selects how a finalized round treats unfinished entries.
type FinalizationPolicy string

// Finalization policies.
const (
	// FinalizationPolicyMarkUnfinishedSkipped converts unfinished entries to
	// skipped on finalization.
	FinalizationPolicyMarkUnfinishedSkipped FinalizationPolicy = "mark_unfinished_skipped"
	// FinalizationPolicyPreserveLastState retains the last entry values while
	// still making them immutable.
	FinalizationPolicyPreserveLastState FinalizationPolicy = "preserve_last_state"
)

// OptOutPolicy selects whether a student opt-out needs manager approval.
type OptOutPolicy string

// Opt-out policies.
const (
	// OptOutPolicyApprovalRequired routes opt-outs through a manager decision.
	OptOutPolicyApprovalRequired OptOutPolicy = "approval_required"
	// OptOutPolicyAutoApprove applies opt-outs directly with no pending state.
	OptOutPolicyAutoApprove OptOutPolicy = "auto_approve"
)

// GradeVisibility selects who may see entry grades and notes.
type GradeVisibility string

// Grade visibility rules.
const (
	// GradeVisibilityManagersAndStudent shows grades to managers and the
	// graded student only.
	GradeVisibilityManagersAndStudent GradeVisibility = "managers_and_student"
	// GradeVisibilityManagersOnly hides grades from students entirely.
	GradeVisibilityManagersOnly GradeVisibility = "managers_only"
	// GradeVisibilityAllParticipants shows grades to everyone in the session.
	GradeVisibilityAllParticipants GradeVisibility = "all_participants"
)

// GradeCorrection selects when a committed grade may still be corrected.
type GradeCorrection string

// Grade correction rules.
const (
	// GradeCorrectionAuditedAnyTime permits audited corrections after commit.
	GradeCorrectionAuditedAnyTime GradeCorrection = "audited_any_time"
	// GradeCorrectionBeforeRoundFinalization permits corrections only while
	// the round is not finalized.
	GradeCorrectionBeforeRoundFinalization GradeCorrection = "before_round_finalization"
	// GradeCorrectionImmutable forbids corrections entirely.
	GradeCorrectionImmutable GradeCorrection = "immutable"
)

// OptOutRequestStatus is the closed status of an opt-out request; it is a
// request status, never an entry status.
type OptOutRequestStatus string

// Opt-out request statuses.
const (
	// OptOutRequestStatusPending awaits a manager decision.
	OptOutRequestStatusPending OptOutRequestStatus = "pending"
	// OptOutRequestStatusApproved records an approved request.
	OptOutRequestStatusApproved OptOutRequestStatus = "approved"
	// OptOutRequestStatusDeclined terminally closes the request leaving the
	// entry waiting (CHK005).
	OptOutRequestStatusDeclined OptOutRequestStatus = "declined"
)

// MaxSurahID is the fixed upper bound of the immutable Quran surah
// reference; quran_surahs holds exactly rows 1..114.
const MaxSurahID = 114

// Round is the durable recitation-queue round (one recitation_queue row).
type Round struct {
	ID              string
	SessionID       string
	RoundNumber     int // positive; sequential per session without reuse
	Type            RoundType
	SurahID         int // 1..MaxSurahID
	FromAyah        int
	ToAyah          int // FromAyah <= ToAyah <= surah ayah count
	GradingRequired bool
	Lifecycle       RoundLifecycle
	SelectedEntryID *string // nil until advance; never means reciting by itself
	Version         int64   // starts at 1; incremented on any queue-visible mutation
	CreatedBy       string
	CreatedAt       time.Time
	ActivatedAt     *time.Time // nil marks a round finalized without ever activating
	FinalizedAt     *time.Time
}

// NewRound contains the immutable input needed to create one durable round.
type NewRound struct {
	SessionID       string
	RoundNumber     int
	Type            RoundType
	SurahID         int
	FromAyah        int
	ToAyah          int
	GradingRequired bool
	Lifecycle       RoundLifecycle
	CreatedBy       string
}

// QueueEntry is one student's entry inside a round.
type QueueEntry struct {
	ID           string
	QueueID      string
	StudentID    string
	Position     int // positive; unique per round
	Status       EntryStatus
	Grade        *Grade  // nil until a completed grading-required entry
	TeacherNotes *string // at most 500 characters when present
	Version      int64   // starts at 1; optimistic mutation guard
	StartedAt    *time.Time
	CompletedAt  *time.Time // set only for completed entries
	ResolvedBy   *string    // manager responsible for the terminal transition
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PreorderCandidate is a pre-activation queue candidate; preparation data,
// not a queue-entry state or practice record.
type PreorderCandidate struct {
	QueueID   string
	StudentID string
	Position  int // positive
	AddedBy   string
	CreatedAt time.Time
}

// QueuePolicy is the five-dimension session queue policy plus its version.
type QueuePolicy struct {
	Population      PopulationPolicy
	Finalization    FinalizationPolicy
	OptOut          OptOutPolicy
	GradeVisibility GradeVisibility
	GradeCorrection GradeCorrection
	Version         int64 // positive; incremented atomically per effective change
}

// OptOutRequest is a student request to leave a round's queue.
type OptOutRequest struct {
	ID             string
	QueueEntryID   string
	RequestedBy    string // must equal the entry's student
	Status         OptOutRequestStatus
	DecidedBy      *string // current teacher/supervisor for decided requests
	RequestedAt    time.Time
	DecidedAt      *time.Time
	IdempotencyKey *string // unique with requester when present
}

// CommandReceipt is the durable idempotency record of one queue command; it
// stores no request body, notes, grade, media data, or response secret.
type CommandReceipt struct {
	SessionID      string
	ActorID        string
	IdempotencyKey string
	Command        string // closed backend command name
	ResourceID     *string
	ResultVersion  *int64 // committed round version
	CreatedAt      time.Time
}

// OutboxEvent is one transactional queue.* event awaiting realtime delivery.
type OutboxEvent struct {
	EventID       string
	SessionID     string
	RoundID       string
	EventType     string // closed queue.* event name
	ResourceID    *string
	RoundVersion  int64
	EventMetadata json.RawMessage // server-built, non-sensitive transition facts only
	AvailableAt   time.Time
	DeliveredAt   *time.Time
	AttemptCount  int        // non-negative
	ParkedAt      *time.Time // set when retries are exhausted
}

// ProgressRecord is the canonical completed-turn practice record (one
// memorization_progress row).
type ProgressRecord struct {
	ID           string
	StudentID    string
	CircleID     string
	SessionID    string
	QueueEntryID string // unique; idempotent completion and re-grade target
	SurahID      int
	SurahName    string // deprecated compat field; all reads use SurahID
	FromAyah     int
	ToAyah       int
	Type         RoundType
	Grade        *Grade  // nil only when the round had no grading requirement
	Notes        *string // at most 500 characters
	Date         time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewProgress contains the completed-turn facts persisted as practice history.
type NewProgress struct {
	StudentID    string
	CircleID     string
	SessionID    string
	QueueEntryID string
	SurahID      int
	SurahName    string
	FromAyah     int
	ToAyah       int
	Type         RoundType
	Grade        *Grade
	Notes        *string
	Date         time.Time
}

// Viewer describes the authorization context for a queue snapshot.
type Viewer struct {
	UserID    string
	IsManager bool
}

// QueueState is the visibility-filtered durable queue snapshot.
type QueueState struct {
	Round    Round
	Entries  []QueueEntry
	Preorder []PreorderCandidate
}

// SessionPolicyContext includes the current queue policy and session facts
// required by service-layer authorization guards.
type SessionPolicyContext struct {
	Policy   QueuePolicy
	Status   string
	CircleID string
}

// QueueErrorCode classifies QueueError values so handlers can map them to
// HTTP statuses without matching on messages.
type QueueErrorCode string

// Queue error classes; stale_version through duplicate_command are command
// conflicts (HTTP 409) and validation is a request-value failure (HTTP 422).
const (
	// QueueErrorCodeValidation means a request value failed a closed-enum,
	// range, length, or positivity check.
	QueueErrorCodeValidation QueueErrorCode = "validation"
	// QueueErrorCodeStaleVersion means the expected optimistic-concurrency
	// version no longer matches the committed row.
	QueueErrorCodeStaleVersion QueueErrorCode = "stale_version"
	// QueueErrorCodeInvalidTransition means the requested entry or round
	// transition is not in the data-model transition table.
	QueueErrorCodeInvalidTransition QueueErrorCode = "invalid_transition"
	// QueueErrorCodeNoWaitingEntry means an advance found no waiting entry.
	QueueErrorCodeNoWaitingEntry QueueErrorCode = "no_waiting_entry"
	// QueueErrorCodeEntryReciting means the entry is the active reciter and
	// cannot take the requested action in its current state.
	QueueErrorCodeEntryReciting QueueErrorCode = "entry_reciting"
	// QueueErrorCodeEntryTerminal means the entry is completed, skipped, or
	// opted_out and cannot change further in this round.
	QueueErrorCodeEntryTerminal QueueErrorCode = "entry_terminal"
	// QueueErrorCodeRoundFinalized means the round is finalized — including
	// a never-activated permanently inert round — and no longer actionable.
	QueueErrorCodeRoundFinalized QueueErrorCode = "round_finalized"
	// QueueErrorCodeDuplicateCommand means an idempotency key was reused
	// with another command.
	QueueErrorCodeDuplicateCommand QueueErrorCode = "duplicate_command"
)

// QueueError is the queue-domain error classification. Message is safe for
// API responses (no internal detail); Err optionally carries the wrapped
// cause for logs. Classify with errors.As against *QueueError.
type QueueError struct {
	Code    QueueErrorCode
	Message string
	Err     error
}

// Error renders the code and safe message plus the wrapped cause, if any.
func (e *QueueError) Error() string {
	if e.Err == nil {
		return string(e.Code) + ": " + e.Message
	}
	return string(e.Code) + ": " + e.Message + ": " + e.Err.Error()
}

// Unwrap exposes the wrapped cause for errors.Is and errors.As.
func (e *QueueError) Unwrap() error { return e.Err }

// entryTransition is one ordered entry-status pair.
type entryTransition struct {
	from EntryStatus
	to   EntryStatus
}

// legalEntryTransitions is the closed entry transition table:
// waiting→reciting→completed, waiting|reciting→skipped, and
// waiting|reciting→opted_out; completed, skipped, and opted_out are terminal.
var legalEntryTransitions = map[entryTransition]struct{}{
	{from: EntryStatusWaiting, to: EntryStatusReciting}:   {},
	{from: EntryStatusReciting, to: EntryStatusCompleted}: {},
	{from: EntryStatusWaiting, to: EntryStatusSkipped}:    {},
	{from: EntryStatusReciting, to: EntryStatusSkipped}:   {},
	{from: EntryStatusWaiting, to: EntryStatusOptedOut}:   {},
	{from: EntryStatusReciting, to: EntryStatusOptedOut}:  {},
}

// CanTransitionEntry reports whether the entry transition from → to is legal
// per the data-model transition table. Terminal statuses never transition.
func CanTransitionEntry(from, to EntryStatus) bool {
	_, ok := legalEntryTransitions[entryTransition{from: from, to: to}]
	return ok
}
