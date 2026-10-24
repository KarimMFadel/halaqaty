package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Create atomically stores one recipient history item and durable delivery intent.
func (r *Repository) Create(ctx context.Context, input CreateInput) (uuid.UUID, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("begin notification creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, inserted, err := CreateInTx(ctx, tx, input)
	if err != nil {
		return uuid.Nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, false, fmt.Errorf("commit notification creation: %w", err)
	}
	return id, inserted, nil
}

// CreateInTx writes history and delivery intent inside the source transition transaction.
func CreateInTx(ctx context.Context, tx pgx.Tx, input CreateInput) (uuid.UUID, bool, error) {
	if !knownEventType(input.Type) {
		return uuid.Nil, false, fmt.Errorf("unknown notification type %q", input.Type)
	}
	if input.PreferenceOverride && input.Type != EventCircleArchived {
		return uuid.Nil, false, errors.New("preference override is valid only for circle archive notices")
	}
	input.SourceEventID = strings.TrimSpace(input.SourceEventID)
	if input.SourceEventID == "" || input.RecipientID == uuid.Nil || input.SourceRef.ID == uuid.Nil ||
		input.Title == "" || input.Body == "" || input.PushExpiresAt.IsZero() {
		return uuid.Nil, false, errors.New("notification projection is incomplete")
	}
	if !knownSourceRefType(input.SourceRef.Type) {
		return uuid.Nil, false, fmt.Errorf("unknown notification source reference type %q", input.SourceRef.Type)
	}

	var createdAt any
	if input.CreatedAt != nil {
		createdAt = input.CreatedAt.UTC()
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, insertNotificationQuery,
		input.RecipientID,
		input.Type,
		input.SourceEventID,
		input.SourceRef.Type,
		input.SourceRef.ID,
		strings.TrimSpace(input.Title),
		strings.TrimSpace(input.Body),
		createdAt,
		input.PushExpiresAt.UTC(),
		input.PreferenceOverride,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingID uuid.UUID
		if err := tx.QueryRow(ctx, findNotificationIDBySourceQuery,
			input.RecipientID, input.Type, input.SourceEventID).Scan(&existingID); err != nil {
			return uuid.Nil, false, fmt.Errorf("resolve replayed notification: %w", err)
		}
		return existingID, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("insert notification projection: %w", err)
	}
	return id, true, nil
}

func knownEventType(eventType EventType) bool {
	for _, preference := range EventPreferences() {
		if preference.Type == eventType {
			return true
		}
	}
	return false
}

func knownSourceRefType(sourceRefType SourceRefType) bool {
	switch sourceRefType {
	case SourceSession, SourceMessage, SourceRecitation, SourceCircle, SourceInvitation, SourceQueue:
		return true
	default:
		return false
	}
}
