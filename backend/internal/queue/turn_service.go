package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const queueEventAdvanced = "queue.advanced"
const queueEventEntryUpdated = "queue.entry_updated"
const queueEventGradeSubmitted = "queue.grade_submitted"

// TurnService owns selection and one-displayed-reciting-entry transitions.
type TurnService struct {
	repo *Repository
}

// NewTurnService constructs a turn service over the durable queue repository.
func NewTurnService(repo *Repository) *TurnService {
	return &TurnService{repo: repo}
}

// Advance selects the next waiting entry without starting it.
func (s *TurnService) Advance(ctx context.Context, roundID string, expectedVersion int64) (Round, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return Round{}, err
	}
	sessionID, err := s.repo.RoundSessionID(ctx, roundID)
	if err != nil {
		return Round{}, err
	}
	var selected Round
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockSession(ctx, sessionID); err != nil {
			return err
		}
		round, err := tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		if round.Lifecycle != RoundLifecycleActive {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		if round.Version != expectedVersion {
			return staleVersionError()
		}
		entries, err := tx.LockEntries(ctx, roundID)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Status == EntryStatusReciting {
				return &QueueError{Code: QueueErrorCodeEntryReciting, Message: "a recitation turn is still active"}
			}
		}
		next, err := tx.SelectNextWaitingEntry(ctx, roundID)
		if err != nil {
			return err
		}
		if next == nil {
			return &QueueError{Code: QueueErrorCodeNoWaitingEntry, Message: "no waiting queue entry"}
		}
		selected, err = tx.SetRoundSelection(ctx, roundID, &next.ID, expectedVersion)
		if err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID,
			ResourceID: &next.ID, EventType: queueEventAdvanced, RoundVersion: selected.Version,
			EventMetadata: json.RawMessage(fmt.Sprintf(`{"selected_entry_id":%q}`, next.ID))})
	})
	return selected, err
}

// Start transitions only the currently selected waiting entry to the displayed
// reciting state. expectedEntryVersion is the entry's own optimistic-lock
// version (EntryStatusRequest.expected_entry_version in the contract); it does
// not change participant audio permission.
func (s *TurnService) Start(ctx context.Context, entryID string, expectedEntryVersion int64) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedEntryVersion); err != nil {
		return QueueEntry{}, err
	}
	var started QueueEntry
	var round Round
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	sessionID, err := s.repo.EntrySessionID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockSession(ctx, sessionID); err != nil {
			return err
		}
		round, err = tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if round.Lifecycle != RoundLifecycleActive {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		if entry.Version != expectedEntryVersion {
			return staleVersionError()
		}
		if round.SelectedEntryID == nil || *round.SelectedEntryID != entryID {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "entry is not selected"}
		}
		if entry.Status != EntryStatusWaiting {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "only the selected waiting entry can start"}
		}
		started, err = tx.TransitionEntry(ctx, entry.ID, EntryStatusWaiting, entry.Version, EntryStatusReciting, nil, nil, nil)
		if err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID, ResourceID: &entry.ID,
			EventType: queueEventEntryUpdated, RoundVersion: round.Version + 1,
			EventMetadata: json.RawMessage(`{"old_status":"waiting","new_status":"reciting"}`)})
	})
	if err != nil {
		return QueueEntry{}, err
	}
	return started, nil
}

// Complete transitions a reciting entry to completed, atomically writing the
// single memorization_progress record for the turn. Grading-required rounds
// require a grade (optional note); grading-optional rounds reject both.
func (s *TurnService) Complete(ctx context.Context, entryID string, expectedEntryVersion int64, resolvedBy string, grade *Grade, notes *string) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedEntryVersion); err != nil {
		return QueueEntry{}, err
	}
	if grade != nil {
		if err := grade.Validate(); err != nil {
			return QueueEntry{}, err
		}
	}
	if notes != nil {
		if err := ValidateNoteLength(*notes); err != nil {
			return QueueEntry{}, err
		}
	}

	var completed QueueEntry
	var round Round
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	sessionID, err := s.repo.EntrySessionID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockSession(ctx, sessionID); err != nil {
			return err
		}
		round, err = tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if round.Lifecycle != RoundLifecycleActive {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		if entry.Version != expectedEntryVersion {
			return staleVersionError()
		}
		if entry.Status != EntryStatusReciting {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "only the reciting entry can be completed"}
		}
		if round.GradingRequired {
			if grade == nil {
				return validationError("grade is required for this round")
			}
		} else {
			if grade != nil || notes != nil {
				return validationError("grade and notes are not allowed for grading-optional rounds")
			}
		}

		completed, err = tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, EntryStatusCompleted, grade, notes, &resolvedBy)
		if err != nil {
			return err
		}

		if err := tx.UpsertProgress(ctx, NewProgress{
			StudentID:    entry.StudentID,
			SessionID:    round.SessionID,
			QueueEntryID: entry.ID,
			SurahID:      round.SurahID,
			SurahName:    "", // derived by UpsertProgress from session circle
			FromAyah:     round.FromAyah,
			ToAyah:       round.ToAyah,
			Type:         round.Type,
			Grade:        grade,
			Notes:        notes,
			Date:         time.Now().UTC(),
		}); err != nil {
			return err
		}

		roundVersion := round.Version + 1
		if round.SelectedEntryID != nil && *round.SelectedEntryID == entry.ID {
			cleared, err := tx.ClearRoundSelection(ctx, round.ID, entry.ID, round.Version)
			if err != nil {
				return err
			}
			roundVersion = cleared.Version
		} else if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}

		metadata := fmt.Sprintf(`{"old_status":"reciting","new_status":"completed","queue_entry_id":%q,"student_id":%q`, entry.ID, entry.StudentID)
		if round.GradingRequired && grade != nil {
			// Grade value is intentionally omitted from metadata (redacted);
			// clients receive visibility-filtered projections via queue.state.
			metadata = metadata + `,"graded":"true"`
		}
		metadata = metadata + "}"
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID, ResourceID: &entry.ID,
			EventType: queueEventGradeSubmitted, RoundVersion: roundVersion,
			EventMetadata: json.RawMessage(metadata)})
	})
	if err != nil {
		return QueueEntry{}, err
	}
	return completed, nil
}

// Correct replaces the grade and/or note of a completed entry under the
// session's grade-correction policy. It updates the same queue entry and the
// same single memorization_progress row atomically and emits a redacted audit
// event. A nil notes pointer leaves the note unchanged; a non-nil empty string
// clears it.
func (s *TurnService) Correct(ctx context.Context, entryID string, expectedEntryVersion int64, resolvedBy string, grade *Grade, notes *string) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedEntryVersion); err != nil {
		return QueueEntry{}, err
	}
	if grade != nil {
		if err := grade.Validate(); err != nil {
			return QueueEntry{}, err
		}
	}
	if notes != nil {
		if err := ValidateNoteLength(*notes); err != nil {
			return QueueEntry{}, err
		}
	}

	var corrected QueueEntry
	var round Round
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	sessionID, err := s.repo.EntrySessionID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		policy, err := tx.LockSessionPolicy(ctx, sessionID)
		if err != nil {
			return err
		}
		round, err = tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.Version != expectedEntryVersion {
			return staleVersionError()
		}
		if entry.Status != EntryStatusCompleted {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "only completed entries can be corrected"}
		}
		switch policy.Policy.GradeCorrection {
		case GradeCorrectionImmutable:
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "grade correction is immutable for this session"}
		case GradeCorrectionBeforeRoundFinalization:
			if round.Lifecycle == RoundLifecycleFinalized {
				return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "round is finalized and no longer accepts corrections"}
			}
		case GradeCorrectionAuditedAnyTime:
			// Corrections allowed regardless of round lifecycle.
		default:
			return validationError("invalid grade correction policy")
		}

		// A nil notes pointer means "leave the existing note unchanged"; an
		// empty string pointer means "clear the note to NULL".
		updateNotes := notes
		if notes == nil {
			updateNotes = entry.TeacherNotes
		} else if *notes == "" {
			updateNotes = nil
		}
		if grade == nil {
			grade = entry.Grade
		}
		corrected, err = tx.UpdateEntryGrade(ctx, entry.ID, entry.Version, grade, updateNotes)
		if err != nil {
			return err
		}
		if err := tx.UpsertProgress(ctx, NewProgress{
			StudentID:    entry.StudentID,
			SessionID:    round.SessionID,
			QueueEntryID: entry.ID,
			SurahID:      round.SurahID,
			SurahName:    "",
			FromAyah:     round.FromAyah,
			ToAyah:       round.ToAyah,
			Type:         round.Type,
			Grade:        corrected.Grade,
			Notes:        corrected.TeacherNotes,
			Date:         time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID, ResourceID: &entry.ID,
			EventType: queueEventGradeSubmitted, RoundVersion: round.Version + 1,
			EventMetadata: json.RawMessage(fmt.Sprintf(`{"old_status":"completed","new_status":"completed","queue_entry_id":%q,"student_id":%q}`, entry.ID, entry.StudentID))})
	})
	if err != nil {
		return QueueEntry{}, err
	}
	return corrected, nil
}

// Skip transitions a waiting or reciting entry to skipped without changing
// participant audio permission. expectedEntryVersion is the entry's own
// optimistic-lock version (EntryStatusRequest.expected_entry_version).
func (s *TurnService) Skip(ctx context.Context, entryID string, expectedEntryVersion int64, resolvedBy string) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedEntryVersion); err != nil {
		return QueueEntry{}, err
	}
	var skipped QueueEntry
	var round Round
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	sessionID, err := s.repo.EntrySessionID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockSession(ctx, sessionID); err != nil {
			return err
		}
		round, err = tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if round.Lifecycle != RoundLifecycleActive {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		if entry.Version != expectedEntryVersion {
			return staleVersionError()
		}
		skipped, err = tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, EntryStatusSkipped, nil, nil, &resolvedBy)
		if err != nil {
			return err
		}
		roundVersion := round.Version + 1
		if round.SelectedEntryID != nil && *round.SelectedEntryID == entry.ID {
			updated, err := tx.ClearRoundSelection(ctx, round.ID, entry.ID, round.Version)
			if err != nil {
				return err
			}
			roundVersion = updated.Version
		} else if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID, ResourceID: &entry.ID,
			EventType: queueEventEntryUpdated, RoundVersion: roundVersion,
			EventMetadata: json.RawMessage(fmt.Sprintf(`{"old_status":%q,"new_status":"skipped"}`, entry.Status))})
	})
	if err != nil {
		return QueueEntry{}, err
	}
	return skipped, nil
}
