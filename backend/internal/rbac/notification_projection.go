package rbac

import (
	"context"
	"fmt"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	circleLifecycleNotificationTTL         = 24 * time.Hour
	studentJoinNotificationRecipientsQuery = `
SELECT cm.user_id
FROM circle_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid AND cm.role = 'teacher' AND u.deleted_at IS NULL
ORDER BY cm.user_id`
	circleArchiveNotificationRecipientsQuery = `
SELECT cm.user_id
FROM circle_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid AND u.deleted_at IS NULL
ORDER BY cm.user_id`
)

// ProjectStudentJoinedCircle creates the teacher history and push intent in
// the same transaction as the accepted membership insertion.
func (r *Repository) ProjectStudentJoinedCircle(ctx context.Context, circleID, studentID string) error {
	if r.notifications == nil {
		return nil
	}
	tx, ok := r.q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("project student circle-join notification: requires a source transaction")
	}
	circleUUID, err := uuid.Parse(circleID)
	if err != nil {
		return fmt.Errorf("project student circle-join notification: parse circle id: %w", err)
	}
	rows, err := tx.Query(ctx, studentJoinNotificationRecipientsQuery, circleUUID)
	if err != nil {
		return fmt.Errorf("project student circle-join notification: list teachers: %w", err)
	}
	recipients, err := scanCircleNotificationRecipients(rows)
	if err != nil {
		return fmt.Errorf("project student circle-join notification: %w", err)
	}
	studentUUID, err := uuid.Parse(studentID)
	if err != nil {
		return fmt.Errorf("project student circle-join notification: parse student id: %w", err)
	}
	now := time.Now().UTC()
	expires := now.Add(circleLifecycleNotificationTTL)
	for _, recipientID := range recipients {
		input := notifications.CreateInput{
			RecipientID:   recipientID,
			Type:          notifications.EventStudentJoinedCircle,
			SourceEventID: "student_joined_circle:" + circleUUID.String() + ":" + studentUUID.String(),
			SourceRef:     notifications.SourceRef{Type: notifications.SourceCircle, ID: circleUUID},
			Title:         "A student joined your circle",
			Body:          "A student joined your circle.",
			CreatedAt:     &now,
			PushExpiresAt: expires,
		}
		if _, _, err := notifications.CreateInTx(ctx, tx, input); err != nil {
			return fmt.Errorf("project student circle-join notification for teacher %s: %w", recipientID, err)
		}
	}
	return nil
}

// ProjectCircleArchived creates one notice per current active member in the
// same transaction as the archive transition.
func (r *Repository) ProjectCircleArchived(ctx context.Context, circleID string) error {
	if r.notifications == nil {
		return nil
	}
	tx, ok := r.q.(pgx.Tx)
	if !ok {
		return fmt.Errorf("project circle archive notifications: requires a source transaction")
	}
	circleUUID, err := uuid.Parse(circleID)
	if err != nil {
		return fmt.Errorf("project circle archive notifications: parse circle id: %w", err)
	}
	rows, err := tx.Query(ctx, circleArchiveNotificationRecipientsQuery, circleUUID)
	if err != nil {
		return fmt.Errorf("project circle archive notifications: list members: %w", err)
	}
	recipients, err := scanCircleNotificationRecipients(rows)
	if err != nil {
		return fmt.Errorf("project circle archive notifications: %w", err)
	}
	now := time.Now().UTC()
	expires := now.Add(circleLifecycleNotificationTTL)
	for _, recipientID := range recipients {
		input := notifications.CreateInput{
			RecipientID:   recipientID,
			Type:          notifications.EventCircleArchived,
			SourceEventID: "circle_archived:" + circleID,
			SourceRef:     notifications.SourceRef{Type: notifications.SourceCircle, ID: circleUUID},
			Title:         "Circle archived",
			Body:          "A circle you joined has been archived.",
			CreatedAt:     &now,
			PushExpiresAt: expires,
		}
		if _, _, err := notifications.CreateInTx(ctx, tx, input); err != nil {
			return fmt.Errorf("project circle archive notification for member %s: %w", recipientID, err)
		}
	}
	return nil
}

func scanCircleNotificationRecipients(rows pgx.Rows) ([]uuid.UUID, error) {
	defer rows.Close()
	var recipients []uuid.UUID
	for rows.Next() {
		var recipientID uuid.UUID
		if err := rows.Scan(&recipientID); err != nil {
			return nil, fmt.Errorf("scan recipient: %w", err)
		}
		recipients = append(recipients, recipientID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipients: %w", err)
	}
	return recipients, nil
}
