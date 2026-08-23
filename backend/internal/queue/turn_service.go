package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/google/uuid"
)

const queueEventAdvanced = "queue.advanced"
const queueEventEntryUpdated = "queue.entry_updated"

// TurnService owns selection and one-reciter-at-a-time entry transitions.
type TurnService struct {
	repo    *Repository
	control sessions.ReciterAudioControl
}

// NewTurnService constructs a turn service with the provider-neutral audio boundary.
func NewTurnService(repo *Repository, control sessions.ReciterAudioControl) *TurnService {
	return &TurnService{repo: repo, control: control}
}

// Advance selects the next waiting entry without starting it.
func (s *TurnService) Advance(ctx context.Context, roundID string, expectedVersion int64) (Round, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return Round{}, err
	}
	var selected Round
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
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

// Start transitions only the currently selected waiting entry to reciting and
// grants audio after the database commit.
func (s *TurnService) Start(ctx context.Context, entryID string, expectedVersion int64) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return QueueEntry{}, err
	}
	var started QueueEntry
	var round Round
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
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
		if round.Version != expectedVersion {
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
	if s.control == nil {
		return started, nil
	}
	if err := s.control.GrantReciterAudio(ctx, round.SessionID, round.ID, started.ID, started.StudentID); err != nil {
		return started, &QueueError{Code: QueueErrorCodeAudioConvergencePending, Message: "recitation audio is still being applied; queue state is saved", Err: err}
	}
	return started, nil
}

// Skip transitions a waiting or reciting entry to skipped and revokes audio
// after the durable transition when the entry was reciting.
func (s *TurnService) Skip(ctx context.Context, entryID string, expectedVersion int64, resolvedBy string) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return QueueEntry{}, err
	}
	var skipped QueueEntry
	var round Round
	revoke := false
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
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
		if round.Version != expectedVersion {
			return staleVersionError()
		}
		if entry.Status == EntryStatusReciting {
			revoke = true
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
	if revoke && s.control != nil {
		if err := s.control.RevokeReciterAudio(ctx, round.SessionID, round.ID, skipped.ID, skipped.StudentID); err != nil {
			return skipped, &QueueError{Code: QueueErrorCodeAudioConvergencePending, Message: "recitation audio is still being applied; queue state is saved", Err: err}
		}
	}
	return skipped, nil
}

// IsAudioConvergencePending reports whether err represents a committed queue
// mutation whose external audio entitlement still needs reconciliation.
func IsAudioConvergencePending(err error) bool {
	var queueErr *QueueError
	return errors.As(err, &queueErr) && queueErr.Code == QueueErrorCodeAudioConvergencePending
}
