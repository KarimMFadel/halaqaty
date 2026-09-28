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

// membershipRow is one current circle membership plus the circle archive flag.
type membershipRow struct {
	role     string
	archived bool
}

// fakeAccessQuerier fakes the pgx boundary for circle-access point lookups:
// it answers only getCircleAccessQuery from an in-memory membership map and
// never materializes a member list.
type fakeAccessQuerier struct {
	memberships map[[2]string]membershipRow
	queries     int
}

func (f *fakeAccessQuerier) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	f.queries++
	if query != getCircleAccessQuery {
		return fakeRow{err: fmt.Errorf("unexpected query: %s", query)}
	}
	key := [2]string{args[0].(string), args[1].(string)}
	member, ok := f.memberships[key]
	if !ok {
		return fakeRow{err: pgx.ErrNoRows}
	}
	return fakeRow{values: []any{member.role, member.archived}}
}

func (f *fakeAccessQuerier) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("authorization reads must not execute writes")
}

// fakeRow is a minimal pgx.Row fake returning canned values or an error.
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan arity: got %d destinations, want %d", len(dest), len(r.values))
	}
	for i, value := range r.values {
		switch target := dest[i].(type) {
		case *string:
			*target = value.(string)
		case *bool:
			*target = value.(bool)
		case *int:
			*target = value.(int)
		case *sql.NullString:
			*target = value.(sql.NullString)
		default:
			return fmt.Errorf("unsupported scan target %T", dest[i])
		}
	}
	return nil
}

const (
	accessCircleID      = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	accessOtherCircleID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	accessTeacherID     = "11111111-1111-1111-1111-111111111111"
	accessSupervisorID  = "22222222-2222-2222-2222-222222222222"
	accessStudentID     = "33333333-3333-3333-3333-333333333333"
	accessOtherMemberID = "44444444-4444-4444-4444-444444444444"
	accessOutsiderID    = "55555555-5555-5555-5555-555555555555"
)

func activeCircleStore() *fakeAccessQuerier {
	return &fakeAccessQuerier{memberships: map[[2]string]membershipRow{
		{accessCircleID, accessTeacherID}:      {role: "teacher"},
		{accessCircleID, accessSupervisorID}:   {role: "supervisor"},
		{accessCircleID, accessStudentID}:      {role: "student"},
		{accessOtherCircleID, accessTeacherID}: {role: "teacher"},
	}}
}

func TestRequireCircleManage_CurrentManagerRolesPass(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()
	for _, userID := range []string{accessTeacherID, accessSupervisorID} {
		access, err := RequireCircleManage(context.Background(), store, accessCircleID, userID)
		if err != nil {
			t.Fatalf("manager %s: unexpected error %v", userID, err)
		}
		if !access.CanManage() || access.Archived {
			t.Fatalf("manager %s: access=%+v, want CanManage and active circle", userID, access)
		}
	}
}

func TestRequireCircleManage_StudentIsDenied(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()

	_, err := RequireCircleManage(context.Background(), store, accessCircleID, accessStudentID)
	if !errors.Is(err, ErrInsufficientCircleRole) {
		t.Fatalf("student manage: got %v, want ErrInsufficientCircleRole", err)
	}
}

func TestCircleAccess_StudentReadsOnlyOwnAttendance(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()

	access, err := RequireCircleRead(context.Background(), store, accessCircleID, accessStudentID)
	if err != nil {
		t.Fatalf("student read: %v", err)
	}
	if !access.CanReadAttendanceOf(accessStudentID) {
		t.Fatal("student must read their own attendance")
	}
	if access.CanReadAttendanceOf(accessOtherMemberID) {
		t.Fatal("student must not read another member's attendance")
	}

	manager, err := RequireCircleRead(context.Background(), store, accessCircleID, accessTeacherID)
	if err != nil {
		t.Fatalf("manager read: %v", err)
	}
	if !manager.CanReadAttendanceOf(accessStudentID) {
		t.Fatal("teacher must read any circle member's attendance")
	}
}

func TestCircleAccess_ArchivedCircleDeniesWritesButPermitsReads(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()
	archived := store.memberships[[2]string{accessCircleID, accessTeacherID}]
	archived.archived = true
	for key := range store.memberships {
		if key[0] == accessCircleID {
			row := store.memberships[key]
			row.archived = true
			store.memberships[key] = row
		}
	}

	access, err := RequireCircleRead(context.Background(), store, accessCircleID, accessTeacherID)
	if err != nil {
		t.Fatalf("archived circle read: %v", err)
	}
	if !access.Archived {
		t.Fatalf("archived flag must surface: %+v", access)
	}

	if _, err := RequireCircleManage(context.Background(), store, accessCircleID, accessTeacherID); !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("archived circle write: got %v, want ErrCircleArchived", err)
	}
	if _, err := RequireCircleManage(context.Background(), store, accessCircleID, accessStudentID); !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("archived circle student write: got %v, want ErrCircleArchived", err)
	}
}

func TestCircleAccess_NonMemberAndRevokedMemberAreDenied(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()

	for name, require := range map[string]func(context.Context, Querier, string, string) (CircleAccess, error){
		"read":   RequireCircleRead,
		"manage": RequireCircleManage,
	} {
		if _, err := require(context.Background(), store, accessCircleID, accessOutsiderID); !errors.Is(err, ErrCircleNotFoundOrDenied) {
			t.Fatalf("non-member %s: got %v, want ErrCircleNotFoundOrDenied", name, err)
		}
	}
	// Revocation deletes the membership row; the point lookup then misses.
	delete(store.memberships, [2]string{accessCircleID, accessStudentID})
	if _, err := RequireCircleRead(context.Background(), store, accessCircleID, accessStudentID); !errors.Is(err, ErrCircleNotFoundOrDenied) {
		t.Fatalf("revoked member read: got %v, want ErrCircleNotFoundOrDenied", err)
	}
}

func TestCircleAccess_CrossCircleDeniedWithoutExistenceDisclosure(t *testing.T) {
	t.Parallel()
	store := activeCircleStore()

	// accessTeacherID manages accessOtherCircleID but has no membership in
	// accessCircleID: the denial must be indistinguishable from "circle does
	// not exist" (404-style), never a role error that confirms the circle.
	_, err := RequireCircleManage(context.Background(), store, accessCircleID, accessSupervisorID+"x")
	if !errors.Is(err, ErrCircleNotFoundOrDenied) {
		t.Fatalf("unknown user on existing circle: got %v, want ErrCircleNotFoundOrDenied", err)
	}
	_, err = RequireCircleManage(context.Background(), store, accessOtherCircleID, accessStudentID)
	if !errors.Is(err, ErrCircleNotFoundOrDenied) {
		t.Fatalf("cross-circle member: got %v, want ErrCircleNotFoundOrDenied (no existence leak)", err)
	}
	if errors.Is(err, ErrInsufficientCircleRole) {
		t.Fatal("cross-circle denial must not surface role information")
	}
}

func TestCircleAccess_IdentityAloneGrantsNothing(t *testing.T) {
	t.Parallel()
	// A caller with a perfectly valid identity but no circle_members row must
	// get nothing: authorization comes from the membership point query, never
	// from the bearer token.
	store := &fakeAccessQuerier{memberships: map[[2]string]membershipRow{}}

	before := store.queries
	if _, err := RequireCircleRead(context.Background(), store, accessCircleID, accessOutsiderID); !errors.Is(err, ErrCircleNotFoundOrDenied) {
		t.Fatalf("identity without membership: got %v, want ErrCircleNotFoundOrDenied", err)
	}
	if got := store.queries - before; got != 1 {
		t.Fatalf("membership check must be one point query, got %d queries", got)
	}
}
