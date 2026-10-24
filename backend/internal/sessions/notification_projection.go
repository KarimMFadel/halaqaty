package sessions

import (
	"context"
	"fmt"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	sessionLifecycleNotificationTTL = 24 * time.Hour
	sessionStartedRecipientsQuery   = `
SELECT cm.user_id
FROM circle_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid AND u.deleted_at IS NULL
ORDER BY cm.user_id`
	sessionEndedRecipientsQuery = `
SELECT DISTINCT sp.user_id
FROM session_participant_presence sp
JOIN users u ON u.id = sp.user_id
WHERE sp.session_id = $1::uuid AND sp.first_joined_at IS NOT NULL
  AND sp.removed_at IS NULL AND u.deleted_at IS NULL
ORDER BY sp.user_id`
)

func (r *Repository) projectSessionStarted(ctx context.Context, q querier, session Session) error {
	if r.notifications == nil {
		return nil
	}
	tx, ok := q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("project session start notification: source transition requires a transaction")
	}
	circleID, err := uuid.Parse(session.CircleID)
	if err != nil {
		return fmt.Errorf("project session start notification: parse circle id: %w", err)
	}
	return r.projectLifecycleNotification(ctx, tx, session, circleID, notifications.EventSessionStarted, sessionStartedRecipientsQuery, "session.started")
}

func (r *Repository) projectSessionEnded(ctx context.Context, q querier, session Session) error {
	if r.notifications == nil {
		return nil
	}
	tx, ok := q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("project session end notification: source transition requires a transaction")
	}
	circleID, err := uuid.Parse(session.CircleID)
	if err != nil {
		return fmt.Errorf("project session end notification: parse circle id: %w", err)
	}
	return r.projectLifecycleNotification(ctx, tx, session, circleID, notifications.EventSessionEnded, sessionEndedRecipientsQuery, "session.ended")
}

func (r *Repository) projectLifecycleNotification(ctx context.Context, tx pgx.Tx, session Session, circleID uuid.UUID, eventType notifications.EventType, recipientsQuery, eventName string) error {
	sessionID, err := uuid.Parse(session.ID)
	if err != nil {
		return fmt.Errorf("project %s notification: parse session id: %w", eventName, err)
	}
	rows, err := tx.Query(ctx, recipientsQuery, map[bool]uuid.UUID{true: circleID, false: sessionID}[eventType == notifications.EventSessionStarted])
	if err != nil {
		return fmt.Errorf("project %s notification: list recipients: %w", eventName, err)
	}
	var recipients []uuid.UUID
	for rows.Next() {
		var recipientID uuid.UUID
		if err := rows.Scan(&recipientID); err != nil {
			rows.Close()
			return fmt.Errorf("project %s notification: scan recipient: %w", eventName, err)
		}
		recipients = append(recipients, recipientID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("project %s notification: iterate recipients: %w", eventName, err)
	}
	rows.Close()
	createdAt := session.ActualStart
	if eventType == notifications.EventSessionEnded {
		createdAt = session.ActualEnd
	}
	if createdAt == nil {
		return fmt.Errorf("project %s notification: transition timestamp is missing", eventName)
	}
	for _, recipientID := range recipients {
		input := notifications.CreateInput{
			RecipientID:   recipientID,
			Type:          eventType,
			SourceEventID: eventName + ":" + session.ID,
			SourceRef:     notifications.SourceRef{Type: notifications.SourceSession, ID: sessionID},
			Title:         "Session update",
			Body:          map[bool]string{true: "A session has started.", false: "A session has ended."}[eventType == notifications.EventSessionStarted],
			CreatedAt:     createdAt,
			PushExpiresAt: createdAt.Add(sessionLifecycleNotificationTTL),
		}
		if _, _, err := notifications.CreateInTx(ctx, tx, input); err != nil {
			return fmt.Errorf("project %s notification: persist recipient %s: %w", eventName, recipientID, err)
		}
	}
	return nil
}
