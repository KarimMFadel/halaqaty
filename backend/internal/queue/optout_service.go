package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const queueEventOptOutRequested = "queue.opt_out_requested"

// OptOutResult carries the committed opt-out request and the affected queue
// entry after a request or decision.
type OptOutResult struct {
	Request OptOutRequest
	Entry   QueueEntry
}

// OptOutAuditSink receives redacted opt-out lifecycle facts after a successful
// database update. ADR-012 keeps the MVP audit sink operational/structured;
// durable queryable audit storage is intentionally deferred.
type OptOutAuditSink interface {
	OptOutRequested(ctx context.Context, actorID, sessionID, entryID string)
	OptOutDecided(ctx context.Context, actorID, sessionID, requestID, decision string)
}

type noopOptOutAuditSink struct{}

func (noopOptOutAuditSink) OptOutRequested(context.Context, string, string, string) {}
func (noopOptOutAuditSink) OptOutDecided(context.Context, string, string, string, string) {
}

// OptOutService owns student opt-out requests and manager decisions.
type OptOutService struct {
	repo  *Repository
	audit OptOutAuditSink
}

// NewOptOutService constructs an opt-out service over the queue repository.
func NewOptOutService(repo *Repository) *OptOutService {
	return NewOptOutServiceWithAudit(repo, nil)
}

// NewOptOutServiceWithAudit constructs an opt-out service with a redacted audit sink.
func NewOptOutServiceWithAudit(repo *Repository, audit OptOutAuditSink) *OptOutService {
	if audit == nil {
		audit = noopOptOutAuditSink{}
	}
	return &OptOutService{repo: repo, audit: audit}
}

// Request resolves the caller's own entry and creates or replays an opt-out
// request under the session's opt-out policy. Under approval_required a pending
// request row is created (idempotent per entry). Under auto_approve the entry
// transitions directly to opted_out with an approved request row and no pending
// state. No memorization_progress or penalty rows are ever written.
func (s *OptOutService) Request(ctx context.Context, sessionID, studentID string) (OptOutResult, error) {
	var result OptOutResult
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		entry, err := tx.FindOwnEntryForSession(ctx, sessionID, studentID)
		if err != nil {
			return err
		}
		existing, err := tx.FindOptOutRequestByEntryAndRequester(ctx, entry.ID, studentID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if existing.Status == OptOutRequestStatusPending || existing.Status == OptOutRequestStatusApproved {
			result = OptOutResult{Request: existing, Entry: entry}
			return nil
		}
		if isTerminalEntryStatus(entry.Status) {
			return &QueueError{Code: QueueErrorCodeEntryTerminal, Message: "entry has already reached a final state"}
		}

		policy, err := tx.LockSessionPolicy(ctx, sessionID)
		if err != nil {
			return err
		}
		if policy.Status != "active" {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "round is finalized and no longer accepts changes"}
		}

		switch policy.Policy.OptOut {
		case OptOutPolicyApprovalRequired:
			result, err = s.requestApprovalRequired(ctx, tx, sessionID, entry, studentID)
		case OptOutPolicyAutoApprove:
			result, err = s.requestAutoApprove(ctx, tx, sessionID, entry, studentID)
		default:
			err = validationError("invalid opt-out policy")
		}
		return err
	})
	if err != nil {
		return OptOutResult{}, err
	}
	return result, nil
}

func (s *OptOutService) requestApprovalRequired(ctx context.Context, tx *Tx, sessionID string, entry QueueEntry, studentID string) (OptOutResult, error) {
	pending, err := tx.FindPendingOptOutRequestByEntry(ctx, entry.ID)
	if err != nil {
		return OptOutResult{}, err
	}
	if pending != nil {
		return OptOutResult{Request: *pending, Entry: entry}, nil
	}
	round, err := tx.LockRound(ctx, entry.QueueID)
	if err != nil {
		return OptOutResult{}, err
	}

	request, err := tx.InsertOptOutRequest(ctx, entry.ID, studentID, OptOutRequestStatusPending, nil)
	if err != nil {
		return OptOutResult{}, err
	}
	s.audit.OptOutRequested(ctx, studentID, sessionID, entry.ID)

	metadata := map[string]string{
		"request_id":     request.ID,
		"queue_entry_id": entry.ID,
		"student_id":     studentID,
	}
	if err := tx.InsertOutboxEvent(ctx, OutboxEvent{
		EventID:       uuid.NewString(),
		SessionID:     sessionID,
		RoundID:       entry.QueueID,
		EventType:     queueEventOptOutRequested,
		ResourceID:    &request.ID,
		RoundVersion:  round.Version,
		EventMetadata: mustEncodeOptOutMetadata(metadata),
	}); err != nil {
		return OptOutResult{}, err
	}
	return OptOutResult{Request: request, Entry: entry}, nil
}

func (s *OptOutService) requestAutoApprove(ctx context.Context, tx *Tx, sessionID string, entry QueueEntry, studentID string) (OptOutResult, error) {
	existing, err := tx.FindOptOutRequestByEntryAndRequester(ctx, entry.ID, studentID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return OptOutResult{}, err
	}
	if existing.Status == OptOutRequestStatusApproved {
		return OptOutResult{Request: existing, Entry: entry}, nil
	}

	updated, err := tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, EntryStatusOptedOut, nil, nil, &studentID)
	if err != nil {
		return OptOutResult{}, err
	}

	round, err := tx.LockRound(ctx, entry.QueueID)
	if err != nil {
		return OptOutResult{}, err
	}
	var roundVersion int64
	if round.SelectedEntryID != nil && *round.SelectedEntryID == entry.ID {
		cleared, err := tx.ClearRoundSelection(ctx, round.ID, entry.ID, round.Version)
		if err != nil {
			return OptOutResult{}, err
		}
		roundVersion = cleared.Version
	} else if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
		return OptOutResult{}, err
	} else {
		roundVersion = round.Version + 1
	}

	var request OptOutRequest
	if existing.ID != "" {
		request = existing
	} else {
		request, err = tx.InsertOptOutRequest(ctx, entry.ID, studentID, OptOutRequestStatusApproved, &studentID)
		if err != nil {
			return OptOutResult{}, err
		}
	}
	s.audit.OptOutDecided(ctx, studentID, sessionID, request.ID, string(OptOutRequestStatusApproved))

	if err := tx.InsertOutboxEvent(ctx, OutboxEvent{
		EventID:       uuid.NewString(),
		SessionID:     sessionID,
		RoundID:       entry.QueueID,
		EventType:     queueEventEntryUpdated,
		ResourceID:    &entry.ID,
		RoundVersion:  roundVersion,
		EventMetadata: json.RawMessage(fmt.Sprintf(`{"old_status":%q,"new_status":"opted_out"}`, entry.Status)),
	}); err != nil {
		return OptOutResult{}, err
	}
	return OptOutResult{Request: request, Entry: updated}, nil
}

// Decide applies a manager decision to one pending opt-out request. Approve
// transitions the entry to opted_out; decline terminally closes the request and
// leaves the entry waiting (CHK005). Finalized rounds and stale entry versions
// are rejected before any mutation; an already-decided request returns a clean
// invalid-transition conflict.
func (s *OptOutService) Decide(ctx context.Context, requestID, managerID string, decision OptOutRequestStatus, expectedEntryVersion int64) (OptOutResult, error) {
	if decision != OptOutRequestStatusApproved && decision != OptOutRequestStatusDeclined {
		return OptOutResult{}, &QueueError{Code: QueueErrorCodeValidation, Message: "invalid decision"}
	}
	if err := ValidateExpectedVersion(expectedEntryVersion); err != nil {
		return OptOutResult{}, err
	}

	var result OptOutResult
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		request, err := tx.LockOptOutRequest(ctx, requestID)
		if err != nil {
			return err
		}
		entry, err := tx.LockEntry(ctx, request.QueueEntryID)
		if err != nil {
			return err
		}
		round, err := tx.LockRound(ctx, entry.QueueID)
		if err != nil {
			return err
		}
		if round.Lifecycle == RoundLifecycleFinalized {
			return &QueueError{Code: QueueErrorCodeRoundFinalized, Message: "round is finalized and no longer accepts changes"}
		}
		if request.Status != OptOutRequestStatusPending {
			return &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "opt-out request is already decided"}
		}
		if entry.Version != expectedEntryVersion {
			return staleVersionError()
		}

		decided, err := tx.DecideOptOutRequest(ctx, requestID, decision, managerID)
		if err != nil {
			return err
		}
		s.audit.OptOutDecided(ctx, managerID, round.SessionID, requestID, string(decision))

		var roundVersion int64
		if decision == OptOutRequestStatusApproved {
			if _, err := tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, EntryStatusOptedOut, nil, nil, &managerID); err != nil {
				return err
			}
			if round.SelectedEntryID != nil && *round.SelectedEntryID == entry.ID {
				cleared, err := tx.ClearRoundSelection(ctx, round.ID, entry.ID, round.Version)
				if err != nil {
					return err
				}
				roundVersion = cleared.Version
			} else if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
				return err
			} else {
				roundVersion = round.Version + 1
			}
			if err := tx.InsertOutboxEvent(ctx, OutboxEvent{
				EventID:       uuid.NewString(),
				SessionID:     round.SessionID,
				RoundID:       round.ID,
				EventType:     queueEventEntryUpdated,
				ResourceID:    &entry.ID,
				RoundVersion:  roundVersion,
				EventMetadata: json.RawMessage(fmt.Sprintf(`{"old_status":%q,"new_status":"opted_out"}`, entry.Status)),
			}); err != nil {
				return err
			}
		}

		finalEntry, err := tx.LockEntry(ctx, entry.ID)
		if err != nil {
			return err
		}
		result = OptOutResult{Request: decided, Entry: finalEntry}
		return nil
	})
	if err != nil {
		return OptOutResult{}, err
	}
	return result, nil
}

func isTerminalEntryStatus(status EntryStatus) bool {
	return status == EntryStatusCompleted || status == EntryStatusSkipped || status == EntryStatusOptedOut
}

func mustEncodeOptOutMetadata(metadata map[string]string) json.RawMessage {
	raw, _ := json.Marshal(metadata)
	return raw
}
