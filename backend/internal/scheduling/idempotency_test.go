package scheduling

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeReplayQuerier fakes the pgx boundary for schedule_request_replays:
// LoadReplay selects and SaveReplay inserts against an in-memory map keyed by
// (actor_id, idempotency_key), mirroring the table's primary key scope.
type fakeReplayQuerier struct {
	rows map[replayScope]ReplayRecord
}

type replayScope struct {
	actorID string
	key     string
}

func newFakeReplayQuerier() *fakeReplayQuerier {
	return &fakeReplayQuerier{rows: map[replayScope]ReplayRecord{}}
}

func (f *fakeReplayQuerier) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	if query != getScheduleRequestReplayQuery {
		return fakeRow{err: fmt.Errorf("unexpected query: %s", query)}
	}
	record, ok := f.rows[replayScope{actorID: args[0].(string), key: args[1].(string)}]
	if !ok {
		return fakeRow{err: pgx.ErrNoRows}
	}
	resource := sql.NullString{}
	if record.ResponseResourceID != nil {
		resource = sql.NullString{String: *record.ResponseResourceID, Valid: true}
	}
	return fakeRow{values: []any{record.ActorID, record.IdempotencyKey, record.CommandFingerprint, record.ResponseStatus, resource}}
}

func (f *fakeReplayQuerier) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if query == lockScheduleRequestReplayQuery {
		return pgconn.NewCommandTag("SELECT 1"), nil
	}
	if query != insertScheduleRequestReplayQuery {
		return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", query)
	}
	scope := replayScope{actorID: args[0].(string), key: args[1].(string)}
	if _, exists := f.rows[scope]; exists {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	resource, _ := args[4].(*string)
	f.rows[scope] = ReplayRecord{
		ActorID:            scope.actorID,
		IdempotencyKey:     scope.key,
		CommandFingerprint: args[2].(string),
		ResponseStatus:     args[3].(int),
		ResponseResourceID: resource,
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

const (
	replayActorA  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	replayActorB  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	replayKey     = "create-schedule-1"
	fingerprintA  = "schedule.create:circle-a:{...v1}"
	fingerprintB  = "schedule.create:circle-a:{...v2}"
	replayRoundID = "99999999-9999-9999-9999-999999999999"
)

func TestReplayOrExecute_FirstExecutionStoresFingerprintAndResponse(t *testing.T) {
	t.Parallel()
	store := newFakeReplayQuerier()
	calls := 0

	record, replayed, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA,
		func(context.Context) (int, *string, error) {
			calls++
			return 201, strPtr(replayRoundID), nil
		})
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}
	if replayed || calls != 1 {
		t.Fatalf("first execution: replayed=%v calls=%d, want false/1", replayed, calls)
	}
	if record.CommandFingerprint != fingerprintA || record.ResponseStatus != 201 {
		t.Fatalf("stored record: %+v, want fingerprint and 201 status", record)
	}
	if record.ResponseResourceID == nil || *record.ResponseResourceID != replayRoundID {
		t.Fatalf("stored resource id: %+v, want %q", record.ResponseResourceID, replayRoundID)
	}

	stored, err := LoadReplay(context.Background(), store, replayActorA, replayKey)
	if err != nil || stored == nil {
		t.Fatalf("replay row must be stored: record=%+v err=%v", stored, err)
	}
	if stored.CommandFingerprint != fingerprintA || stored.ResponseStatus != 201 {
		t.Fatalf("persisted record: %+v", stored)
	}
}

func TestReplayOrExecute_SameKeyAndFingerprintReplaysWithoutReexecuting(t *testing.T) {
	t.Parallel()
	store := newFakeReplayQuerier()
	execute := func(context.Context) (int, *string, error) { return 200, strPtr(replayRoundID), nil }
	if _, _, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA, execute); err != nil {
		t.Fatalf("first execution: %v", err)
	}

	calls := 0
	record, replayed, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA,
		func(context.Context) (int, *string, error) {
			calls++
			return 500, nil, nil
		})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed || calls != 0 {
		t.Fatalf("replay: replayed=%v calls=%d, want true/0 (stored response, no re-execution)", replayed, calls)
	}
	if record.ResponseStatus != 200 {
		t.Fatalf("replayed status: got %d, want stored 200", record.ResponseStatus)
	}
	if record.ResponseResourceID == nil || *record.ResponseResourceID != replayRoundID {
		t.Fatalf("replayed resource: %+v, want stored %q", record.ResponseResourceID, replayRoundID)
	}
}

func TestReplayOrExecute_SameKeyDifferentFingerprintConflicts(t *testing.T) {
	t.Parallel()
	store := newFakeReplayQuerier()
	if _, _, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA,
		func(context.Context) (int, *string, error) { return 201, strPtr(replayRoundID), nil }); err != nil {
		t.Fatalf("first execution: %v", err)
	}

	calls := 0
	_, _, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintB,
		func(context.Context) (int, *string, error) {
			calls++
			return 201, nil, nil
		})
	if !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("fingerprint mismatch: got %v, want ErrReplayConflict (409-style)", err)
	}
	if calls != 0 {
		t.Fatalf("conflicting replay must not re-execute, got %d calls", calls)
	}
}

func TestReplayOrExecute_DifferentActorIsAnIndependentScope(t *testing.T) {
	t.Parallel()
	store := newFakeReplayQuerier()
	if _, _, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA,
		func(context.Context) (int, *string, error) { return 201, strPtr(replayRoundID), nil }); err != nil {
		t.Fatalf("actor A first execution: %v", err)
	}

	calls := 0
	record, replayed, err := ReplayOrExecute(context.Background(), store, replayActorB, replayKey, fingerprintB,
		func(context.Context) (int, *string, error) {
			calls++
			return 200, nil, nil
		})
	if err != nil {
		t.Fatalf("actor B must have an independent key scope: %v", err)
	}
	if replayed || calls != 1 {
		t.Fatalf("actor B: replayed=%v calls=%d, want false/1", replayed, calls)
	}
	if record.ActorID != replayActorB || record.CommandFingerprint != fingerprintB {
		t.Fatalf("actor B record: %+v", record)
	}
}

func TestReplayOrExecute_ExecutionFailureStoresNothing(t *testing.T) {
	t.Parallel()
	store := newFakeReplayQuerier()
	boom := errors.New("command failed")

	_, _, err := ReplayOrExecute(context.Background(), store, replayActorA, replayKey, fingerprintA,
		func(context.Context) (int, *string, error) { return 0, nil, boom })
	if !errors.Is(err, boom) {
		t.Fatalf("execution error must propagate: got %v, want wrapped %v", err, boom)
	}

	stored, loadErr := LoadReplay(context.Background(), store, replayActorA, replayKey)
	if loadErr != nil || stored != nil {
		t.Fatalf("failed execution must not store a replay row: record=%+v err=%v", stored, loadErr)
	}
}

func strPtr(value string) *string { return &value }
