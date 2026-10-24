package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrRecipientNotFound indicates the account is missing or closed.
	ErrRecipientNotFound = errors.New("notification recipient not found")
	// ErrPreferenceNotConfigurable indicates an always-on or unknown type was changed.
	ErrPreferenceNotConfigurable = errors.New("notification type is not configurable")
	// ErrInvalidQuietHours indicates malformed or equal local clock values.
	ErrInvalidQuietHours = errors.New("invalid notification quiet hours")
)

// GetPreferences returns effective values, including defaults for unset rows.
func (r *Repository) GetPreferences(ctx context.Context, userID string) (Preferences, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Preferences{}, fmt.Errorf("begin notification preference read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result Preferences
	err = tx.QueryRow(ctx, getNotificationPreferencesQuery, userID).Scan(
		&result.QuietHours.Timezone,
		&result.QuietHours.Enabled,
		&result.QuietHours.StartLocal,
		&result.QuietHours.EndLocal,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preferences{}, ErrRecipientNotFound
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("get notification preference defaults: %w", err)
	}

	settings := make(map[EventType]bool)
	rows, err := tx.Query(ctx, listNotificationTypePreferencesQuery, userID)
	if err != nil {
		return Preferences{}, fmt.Errorf("list notification type preferences: %w", err)
	}
	for rows.Next() {
		var eventType EventType
		var enabled bool
		if err := rows.Scan(&eventType, &enabled); err != nil {
			rows.Close()
			return Preferences{}, fmt.Errorf("scan notification type preference: %w", err)
		}
		settings[eventType] = enabled
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Preferences{}, fmt.Errorf("iterate notification type preferences: %w", err)
	}
	rows.Close()

	for _, preference := range EventPreferences() {
		enabled, exists := settings[preference.Type]
		if !exists {
			enabled = preference.DefaultEnabled
		}
		result.Types = append(result.Types, TypeSetting{
			Type:           preference.Type,
			Enabled:        enabled,
			Configurable:   preference.Configurable,
			DefaultEnabled: preference.DefaultEnabled,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return Preferences{}, fmt.Errorf("commit notification preference read: %w", err)
	}
	return result, nil
}

// UpdatePreferences atomically stores only the supplied configurable values.
func (r *Repository) UpdatePreferences(ctx context.Context, userID string, types []TypeSettingUpdate, quietHours *QuietHoursUpdate) error {
	seen := make(map[EventType]struct{}, len(types))
	for _, update := range types {
		if _, duplicate := seen[update.Type]; duplicate {
			return fmt.Errorf("duplicate notification preference %q: %w", update.Type, ErrPreferenceNotConfigurable)
		}
		seen[update.Type] = struct{}{}
		if !isConfigurable(update.Type) {
			return fmt.Errorf("notification preference %q: %w", update.Type, ErrPreferenceNotConfigurable)
		}
	}
	if quietHours != nil {
		if _, err := parseLocalClock(quietHours.StartLocal); err != nil {
			return fmt.Errorf("quiet-hours start: %w", err)
		}
		if _, err := parseLocalClock(quietHours.EndLocal); err != nil {
			return fmt.Errorf("quiet-hours end: %w", err)
		}
		if quietHours.StartLocal == quietHours.EndLocal {
			return ErrInvalidQuietHours
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin notification preference update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, update := range types {
		if _, err := tx.Exec(ctx, upsertNotificationTypePreferenceQuery, userID, update.Type, update.Enabled); err != nil {
			return fmt.Errorf("update notification type preference: %w", err)
		}
	}
	if quietHours != nil {
		if _, err := tx.Exec(ctx, upsertNotificationQuietHoursQuery,
			userID, quietHours.Enabled, quietHours.StartLocal, quietHours.EndLocal); err != nil {
			return fmt.Errorf("update notification quiet hours: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit notification preference update: %w", err)
	}
	return nil
}

// IsQuietHours reports whether now falls in the enabled interval in its IANA zone.
func IsQuietHours(quiet QuietHours, now time.Time) (bool, error) {
	if !quiet.Enabled {
		return false, nil
	}
	start, err := parseLocalClock(quiet.StartLocal)
	if err != nil {
		return false, fmt.Errorf("parse quiet-hours start: %w", err)
	}
	end, err := parseLocalClock(quiet.EndLocal)
	if err != nil {
		return false, fmt.Errorf("parse quiet-hours end: %w", err)
	}
	if start == end {
		return false, ErrInvalidQuietHours
	}
	location, err := time.LoadLocation(quiet.Timezone)
	if err != nil {
		return false, fmt.Errorf("load profile timezone: %w", err)
	}
	local := now.In(location)
	minute := local.Hour()*60 + local.Minute()
	if start < end {
		return minute >= start && minute < end, nil
	}
	return minute >= start || minute < end, nil
}

func isConfigurable(eventType EventType) bool {
	for _, preference := range EventPreferences() {
		if preference.Type == eventType {
			return preference.Configurable
		}
	}
	return false
}

func parseLocalClock(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, ErrInvalidQuietHours
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}
