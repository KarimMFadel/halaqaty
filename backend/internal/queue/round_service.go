package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	queueEventRoundStarted   = "queue.round_started"
	queueEventReordered      = "queue.reordered"
	queueEventRoundFinalized = "queue.round_finalized"
)

// RoundService owns preparation, automatic activation, durable ordering, and reset.
type RoundService struct {
	repo    *Repository
	control sessions.ReciterAudioControl
}

// NewRoundService constructs a round service over the queue repository.
func NewRoundService(repo *Repository, control sessions.ReciterAudioControl) *RoundService {
	return &RoundService{repo: repo, control: control}
}

// PrepareRoundInput contains the immutable facts needed to prepare a round.
type PrepareRoundInput struct {
	SessionID       string
	Type            RoundType
	SurahID         int
	FromAyah        int
	ToAyah          int
	SurahAyahCount  int
	GradingRequired bool
	CreatedBy       string
	Preorder        []string
}

// Prepare creates a prepared round and restores the activation invariant when
// the session is already live. Prepared rounds are never activated out of order.
func (s *RoundService) Prepare(ctx context.Context, in PrepareRoundInput) (Round, error) {
	if err := validatePrepareInput(in); err != nil {
		return Round{}, err
	}
	var created Round
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, in.SessionID); err != nil {
			return err
		}
		policy, err := tx.LockSessionPolicy(ctx, in.SessionID)
		if err != nil {
			return err
		}
		if policy.Status != "scheduled" && policy.Status != "active" {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "session is not accepting queue rounds"}
		}
		number, err := tx.NextRoundNumber(ctx, in.SessionID)
		if err != nil {
			return err
		}
		created, err = tx.CreateRound(ctx, NewRound{SessionID: in.SessionID, RoundNumber: number,
			Type: in.Type, SurahID: in.SurahID, FromAyah: in.FromAyah, ToAyah: in.ToAyah,
			GradingRequired: in.GradingRequired, Lifecycle: RoundLifecyclePrepared, CreatedBy: in.CreatedBy})
		if err != nil {
			return err
		}
		if len(in.Preorder) > 0 {
			if err := tx.ReplacePreorder(ctx, created.ID, in.CreatedBy, in.Preorder); err != nil {
				return err
			}
		}
		if policy.Status == "scheduled" {
			return nil
		}
		activated, err := s.activateLowest(ctx, tx, in.SessionID, policy)
		if err != nil {
			return err
		}
		if activated.ID == created.ID {
			created = activated
		}
		return nil
	})
	if err != nil {
		return Round{}, err
	}
	return created, nil
}

func validatePrepareInput(in PrepareRoundInput) error {
	if err := in.Type.Validate(); err != nil {
		return err
	}
	if err := ValidateQuranRange(in.SurahID, in.FromAyah, in.ToAyah, in.SurahAyahCount); err != nil {
		return err
	}
	if in.CreatedBy == "" || in.SessionID == "" {
		return validationError("queue identifiers are required")
	}
	return validatePreorderIDs(in.Preorder)
}

func validatePreorderIDs(studentIDs []string) error {
	seen := make(map[string]struct{}, len(studentIDs))
	for _, id := range studentIDs {
		if _, ok := seen[id]; ok {
			return validationError("preorder contains a duplicate student")
		}
		if id == "" {
			return validationError("preorder contains an invalid student id")
		}
		seen[id] = struct{}{}
		if _, err := uuid.Parse(id); err != nil {
			return validationError("preorder contains an invalid student id")
		}
	}
	return nil
}

func (s *RoundService) activateLowest(ctx context.Context, tx *Tx, sessionID string, policy SessionPolicyContext) (Round, error) {
	if _, err := tx.LockActiveRound(ctx, sessionID); err == nil {
		return Round{}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Round{}, err
	}
	round, err := tx.LockLowestPreparedRound(ctx, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Round{}, nil
		}
		return Round{}, err
	}
	students, err := tx.PopulationOrder(ctx, sessionID, round.ID, policy.Policy.Population)
	if err != nil {
		return Round{}, err
	}
	for position, studentID := range students {
		if _, err := tx.InsertQueueEntry(ctx, round.ID, studentID, position+1); err != nil {
			return Round{}, err
		}
	}
	activated, err := tx.ActivateRound(ctx, round.ID)
	if err != nil {
		return Round{}, err
	}
	if err := tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: sessionID,
		RoundID: round.ID, EventType: queueEventRoundStarted, RoundVersion: activated.Version,
		EventMetadata: json.RawMessage(fmt.Sprintf(`{"round_number":%d,"round_type":%q}`, activated.RoundNumber, activated.Type))}); err != nil {
		return Round{}, err
	}
	return activated, nil
}

// ActivateIfNeeded restores the live-session activation invariant.
func (s *RoundService) ActivateIfNeeded(ctx context.Context, sessionID string) error {
	return s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, sessionID); err != nil {
			return err
		}
		policy, err := tx.LockSessionPolicy(ctx, sessionID)
		if err != nil {
			return err
		}
		if policy.Status == "scheduled" {
			return nil
		}
		if policy.Status != "active" {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "session is not active"}
		}
		_, err = s.activateLowest(ctx, tx, sessionID, policy)
		return err
	})
}

// Reorder replaces the complete pre-activation candidate order.
func (s *RoundService) Reorder(ctx context.Context, roundID, actorID string, expectedVersion int64, studentIDs []string) (Round, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return Round{}, err
	}
	if err := validatePreorderIDs(studentIDs); err != nil {
		return Round{}, err
	}
	var updated Round
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		round, err := tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		if round.Lifecycle != RoundLifecyclePrepared {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "queue order is no longer editable"}
		}
		if round.Version != expectedVersion {
			return staleVersionError()
		}
		if err := tx.ReplacePreorder(ctx, roundID, actorID, studentIDs); err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, roundID); err != nil {
			return err
		}
		updated, err = tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID,
			EventType: queueEventReordered, RoundVersion: updated.Version, EventMetadata: json.RawMessage(`{"order_kind":"preorder_students"}`)})
	})
	return updated, err
}

// Move repositions one waiting entry in an active round.
func (s *RoundService) Move(ctx context.Context, entryID string, expectedVersion int64, newPosition int) (QueueEntry, error) {
	if err := ValidateExpectedVersion(expectedVersion); err != nil {
		return QueueEntry{}, err
	}
	var moved QueueEntry
	roundID, err := s.repo.EntryQueueID(ctx, entryID)
	if err != nil {
		return QueueEntry{}, err
	}
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		round, err := tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.Status != EntryStatusWaiting {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "only waiting entries can move"}
		}
		if round.Lifecycle != RoundLifecycleActive {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "only active queue entries can move"}
		}
		if round.Version != expectedVersion {
			return staleVersionError()
		}
		if err := tx.RepositionEntry(ctx, round.ID, entry.ID, newPosition); err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		moved, err = tx.LockEntry(ctx, entry.ID)
		if err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: round.SessionID, RoundID: round.ID,
			EventType: queueEventReordered, ResourceID: &entry.ID, RoundVersion: round.Version + 1,
			EventMetadata: json.RawMessage(`{"order_kind":"entry_move"}`)})
	})
	return moved, err
}

// Reset finalizes the active round, revokes its audio entitlement, and then
// activates the lowest prepared round.
func (s *RoundService) Reset(ctx context.Context, in PrepareRoundInput) (Round, error) {
	if err := validatePrepareInput(in); err != nil {
		return Round{}, err
	}
	var next Round
	err := s.repo.withSessionRoundLock(ctx, in.SessionID, func(runTx queueTxRunner) error {
		var reciter QueueEntry
		revoke := false
		if err := runTx(ctx, func(tx *Tx) error {
			policy, err := tx.LockSessionPolicy(ctx, in.SessionID)
			if err != nil {
				return err
			}
			if policy.Status != "active" {
				return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "session is not active"}
			}
			active, err := tx.LockActiveRound(ctx, in.SessionID)
			if err != nil {
				return err
			}
			if active.SelectedEntryID != nil {
				selected, err := tx.LockEntry(ctx, *active.SelectedEntryID)
				if err != nil {
					return err
				}
				if selected.Status == EntryStatusReciting {
					reciter = selected
					revoke = true
				}
			}
			finalized, err := tx.FinalizeRound(ctx, active.ID, active.Version, policy.Policy.Finalization, in.CreatedBy)
			if err != nil {
				return err
			}
			if err := tx.InsertOutboxEvent(ctx, OutboxEvent{EventID: uuid.NewString(), SessionID: in.SessionID, RoundID: active.ID,
				EventType: queueEventRoundFinalized, RoundVersion: finalized.Version, EventMetadata: json.RawMessage(`{"reason":"reset"}`)}); err != nil {
				return err
			}
			number, err := tx.NextRoundNumber(ctx, in.SessionID)
			if err != nil {
				return err
			}
			next, err = tx.CreateRound(ctx, NewRound{SessionID: in.SessionID, RoundNumber: number, Type: in.Type, SurahID: in.SurahID,
				FromAyah: in.FromAyah, ToAyah: in.ToAyah, GradingRequired: in.GradingRequired, Lifecycle: RoundLifecyclePrepared, CreatedBy: in.CreatedBy})
			if err != nil {
				return err
			}
			if len(in.Preorder) > 0 {
				if err := tx.ReplacePreorder(ctx, next.ID, in.CreatedBy, in.Preorder); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if revoke && s.control != nil {
			if err := s.control.RevokeReciterAudio(ctx, in.SessionID, reciter.QueueID, reciter.ID, reciter.StudentID); err != nil {
				return &QueueError{Code: QueueErrorCodeAudioConvergencePending, Message: "recitation audio is still being applied; queue state is saved", Err: err}
			}
		}
		return runTx(ctx, func(tx *Tx) error {
			policy, err := tx.LockSessionPolicy(ctx, in.SessionID)
			if err != nil {
				return err
			}
			if policy.Status != "active" {
				return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "session is not active"}
			}
			activated, err := s.activateLowest(ctx, tx, in.SessionID, policy)
			if err != nil {
				return err
			}
			if activated.ID == next.ID {
				next = activated
			}
			return nil
		})
	})
	if err != nil {
		return next, err
	}
	return next, nil
}
