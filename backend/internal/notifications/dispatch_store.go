package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Repository implements DispatchStore on PostgreSQL.
var _ DispatchStore = (*Repository)(nil)

// ExpireDueIntents terminally expires a bounded batch of pending intents past
// their push deadline and returns how many were swept.
func (r *Repository) ExpireDueIntents(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := r.pool.Exec(ctx, expireDueIntentsQuery, now.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("expire due notification intents: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ClaimDueIntents leases up to limit due pending intents: claimed rows are
// leased until now+dispatchLeaseInterval so concurrent dispatchers cannot
// re-claim them while the lease holds, and a crashed dispatcher's rows become
// claimable again once the lease expires. Delivery is at-least-once: a crash
// between provider acceptance and persistence re-sends on the next claim.
// Lifecycle updates settle the lease by moving rows to a terminal state or
// scheduling the next retry.
func (r *Repository) ClaimDueIntents(ctx context.Context, now time.Time, limit int) ([]DispatchIntent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin notification intent claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, claimDueIntentsQuery, now.UTC(), limit, now.UTC().Add(dispatchLeaseInterval))
	if err != nil {
		return nil, fmt.Errorf("claim due notification intents: %w", err)
	}
	intents := make([]DispatchIntent, 0, limit)
	for rows.Next() {
		var intent DispatchIntent
		if err := rows.Scan(
			&intent.ID,
			&intent.UserID,
			&intent.Type,
			&intent.SourceEventID,
			&intent.SourceRef.Type,
			&intent.SourceRef.ID,
			&intent.Title,
			&intent.Body,
			&intent.PushExpiresAt,
			&intent.PreferenceOverride,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan claimed notification intent: %w", err)
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate claimed notification intents: %w", err)
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit notification intent claim: %w", err)
	}
	return intents, nil
}

// HasDeviceAttempts reports whether any per-device attempt row exists, which
// decides whether the first-attempt dispatch gates still apply.
func (r *Repository) HasDeviceAttempts(ctx context.Context, notificationID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, hasDeviceAttemptsQuery, notificationID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check notification device attempts: %w", err)
	}
	return exists, nil
}

// LoadDispatchGates resolves the current type preference (matrix default ON
// when no row exists) and the quiet-hours configuration with the profile IANA
// timezone, reusing the T008 preference resolution.
func (r *Repository) LoadDispatchGates(ctx context.Context, userID uuid.UUID, eventType EventType) (DispatchGates, error) {
	var gates DispatchGates
	err := r.pool.QueryRow(ctx, getNotificationPreferencesQuery, userID).Scan(
		&gates.QuietHours.Timezone,
		&gates.QuietHours.Enabled,
		&gates.QuietHours.StartLocal,
		&gates.QuietHours.EndLocal,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DispatchGates{}, ErrRecipientNotFound
	}
	if err != nil {
		return DispatchGates{}, fmt.Errorf("load quiet-hours gate: %w", err)
	}

	gates.PreferenceEnabled = defaultPreferenceEnabled(eventType)
	var stored bool
	err = r.pool.QueryRow(ctx, getTypePreferenceEnabledQuery, userID, eventType).Scan(&stored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Missing rows resolve to the matrix default.
	case err != nil:
		return DispatchGates{}, fmt.Errorf("load type preference gate: %w", err)
	default:
		gates.PreferenceEnabled = stored
	}
	return gates, nil
}

// ListDueDeviceTargets returns the recipient's registered devices whose
// per-device attempt for this notification is due at now.
func (r *Repository) ListDueDeviceTargets(ctx context.Context, notificationID, userID uuid.UUID, now time.Time) ([]DeviceTarget, error) {
	rows, err := r.pool.Query(ctx, listDueDeviceTargetsQuery, notificationID, userID, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("list due device targets: %w", err)
	}
	defer rows.Close()
	targets := make([]DeviceTarget, 0, 2)
	for rows.Next() {
		var target DeviceTarget
		if err := rows.Scan(&target.DeviceTokenID, &target.Token, &target.AttemptCount); err != nil {
			return nil, fmt.Errorf("scan due device target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due device targets: %w", err)
	}
	return targets, nil
}

// SuppressIntent terminally suppresses the occurrence. Preference and
// quiet-hours suppression also record the same per-device suppression outcome
// for every currently registered device; a DeliveryExpired suppression (the
// volatile source-eligibility recheck) leaves attempt rows untouched — exactly
// like the expiry sweep, a terminal intent is never reclaimed and orphaned
// pending attempt rows are removed by retention cleanup. The guarded update
// never revives an already-terminal row, so re-enabling the preference or a
// later source change cannot replay the occurrence. It reports whether the
// notification row actually transitioned (false when it was already terminal),
// so callers only count real suppressions.
func (r *Repository) SuppressIntent(ctx context.Context, notificationID, userID uuid.UUID, state DeliveryState) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin notification suppression: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if state != DeliveryExpired {
		if _, err := tx.Exec(ctx, suppressDeviceAttemptsQuery, notificationID, string(state), userID); err != nil {
			return false, fmt.Errorf("record per-device suppression: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, suppressNotificationQuery, notificationID, string(state))
	if err != nil {
		return false, fmt.Errorf("suppress notification intent: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit notification suppression: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// RecordAccepted marks one device attempt provider-accepted.
func (r *Repository) RecordAccepted(ctx context.Context, notificationID, deviceTokenID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, recordAcceptedAttemptQuery, notificationID, deviceTokenID); err != nil {
		return fmt.Errorf("record accepted device attempt: %w", err)
	}
	return nil
}

// RecordRetry reschedules one failed attempt with its backoff deadline and
// safe error class, or parks it in the terminal exhausted state.
func (r *Repository) RecordRetry(ctx context.Context, notificationID, deviceTokenID uuid.UUID, nextAttemptAt time.Time, errorClass string, exhausted bool) error {
	state := "pending"
	if exhausted {
		state = "exhausted"
	}
	if _, err := r.pool.Exec(ctx, recordRetryAttemptQuery, notificationID, deviceTokenID, nextAttemptAt.UTC(), errorClass, state); err != nil {
		return fmt.Errorf("record failed device attempt: %w", err)
	}
	return nil
}

// SettleNotification recomputes the aggregate delivery state from per-device
// outcomes and returns it. An already-terminal notification keeps its state.
func (r *Repository) SettleNotification(ctx context.Context, notificationID uuid.UUID) (DeliveryState, error) {
	var state DeliveryState
	err := r.pool.QueryRow(ctx, settleNotificationQuery, notificationID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := r.pool.QueryRow(ctx, getDeliveryStateQuery, notificationID).Scan(&state); err != nil {
			return "", fmt.Errorf("read settled delivery state: %w", err)
		}
		return state, nil
	}
	if err != nil {
		return "", fmt.Errorf("settle notification delivery state: %w", err)
	}
	return state, nil
}

func defaultPreferenceEnabled(eventType EventType) bool {
	for _, preference := range EventPreferences() {
		if preference.Type == eventType {
			return preference.DefaultEnabled
		}
	}
	return true
}
