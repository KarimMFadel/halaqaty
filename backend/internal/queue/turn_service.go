package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const queueEventAdvanced = "queue.advanced"
const queueEventEntryUpdated = "queue.entry_updated"

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
