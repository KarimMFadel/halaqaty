package scheduling

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrReplayConflict means the key was reused for different command input.
var ErrReplayConflict = errors.New("idempotency key already used for different input")

// ReplayRecord is the durable response identity of one actor's command.
type ReplayRecord struct {
	ActorID            string
	IdempotencyKey     string
	CommandFingerprint string
	ResponseStatus     int
	ResponseResourceID *string
}

// LoadReplay reads a stored command result.
func LoadReplay(ctx context.Context, tx Querier, actorID, key string) (*ReplayRecord, error) {
	var record ReplayRecord
	var resource sql.NullString
	err := tx.QueryRow(ctx, getScheduleRequestReplayQuery, actorID, key).Scan(
		&record.ActorID, &record.IdempotencyKey, &record.CommandFingerprint,
		&record.ResponseStatus, &resource,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load schedule replay: %w", err)
	}
	if resource.Valid {
		record.ResponseResourceID = &resource.String
	}
	return &record, nil
}

// ReplayOrExecute runs inside the caller's transaction. The advisory transaction
// lock serializes the same actor/key; commit mutation and replay together.
func ReplayOrExecute(ctx context.Context, tx Querier, actorID, key, fingerprint string, execute func(context.Context) (int, *string, error)) (ReplayRecord, bool, error) {
	if _, err := tx.Exec(ctx, lockScheduleRequestReplayQuery, actorID, key); err != nil {
		return ReplayRecord{}, false, fmt.Errorf("lock schedule replay: %w", err)
	}
	prior, err := LoadReplay(ctx, tx, actorID, key)
	if err != nil {
		return ReplayRecord{}, false, err
	}
	if prior != nil {
		if prior.CommandFingerprint != fingerprint {
			return ReplayRecord{}, false, ErrReplayConflict
		}
		return *prior, true, nil
	}
	status, resource, err := execute(ctx)
	if err != nil {
		return ReplayRecord{}, false, fmt.Errorf("execute schedule command: %w", err)
	}
	record := ReplayRecord{ActorID: actorID, IdempotencyKey: key, CommandFingerprint: fingerprint, ResponseStatus: status, ResponseResourceID: resource}
	result, err := tx.Exec(ctx, insertScheduleRequestReplayQuery, actorID, key, fingerprint, status, resource)
	if err != nil {
		return ReplayRecord{}, false, fmt.Errorf("store schedule replay: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ReplayRecord{}, false, ErrReplayConflict
	}
	return record, false, nil
}
