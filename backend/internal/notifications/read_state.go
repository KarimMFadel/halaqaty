package notifications

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNotificationNotFound indicates the notification is unknown to the
// recipient. Unknown and foreign identifiers are indistinguishable so that
// recipients cannot probe other users' history (no disclosure).
var ErrNotificationNotFound = errors.New("notification not found")

// MarkRead marks one recipient-owned notification read and returns the
// authoritative unread count from the same transaction. Re-marking an
// already-read notification is an idempotent success.
func (r *Repository) MarkRead(ctx context.Context, userID string, notificationID uuid.UUID) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("begin notification mark-read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, markNotificationReadQuery, notificationID, userID)
	if err != nil {
		return 0, fmt.Errorf("mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, notificationExistsForUserQuery, notificationID, userID).Scan(&exists); err != nil {
			return 0, fmt.Errorf("check notification ownership: %w", err)
		}
		if !exists {
			return 0, ErrNotificationNotFound
		}
	}

	var unread int64
	if err := tx.QueryRow(ctx, countUnreadNotificationsQuery, userID).Scan(&unread); err != nil {
		return 0, fmt.Errorf("count unread notifications after mark-read: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit notification mark-read: %w", err)
	}
	return unread, nil
}

// MarkAllRead atomically marks every in-window notification read and returns
// the authoritative unread count from the same transaction. The update shares
// the 30-day window with the unread count so both stay synchronized.
func (r *Repository) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("begin notification mark-all-read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, markAllNotificationsReadQuery, userID); err != nil {
		return 0, fmt.Errorf("mark all notifications read: %w", err)
	}

	var unread int64
	if err := tx.QueryRow(ctx, countUnreadNotificationsQuery, userID).Scan(&unread); err != nil {
		return 0, fmt.Errorf("count unread notifications after mark-all-read: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit notification mark-all-read: %w", err)
	}
	return unread, nil
}
