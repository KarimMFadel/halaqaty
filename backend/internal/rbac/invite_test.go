package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestGenerateInviteCode_Format(t *testing.T) {
	for range 100 {
		code, err := generateInviteCode()
		if err != nil {
			t.Fatal(err)
		}
		if !isInviteCode(code) {
			t.Fatalf("invalid invite code %q", code)
		}
	}
}

func TestCreateCircle_StopsAfterInviteCollisionLimit(t *testing.T) {
	store := newStubStore()
	store.insertErrors = []error{
		&pgconn.PgError{Code: "23505"},
		&pgconn.PgError{Code: "23505"},
		&pgconn.PgError{Code: "23505"},
	}
	_, err := NewService(store, nil).CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{Name: "Circle"})
	if err == nil || !errors.As(err, new(*pgconn.PgError)) {
		t.Fatalf("expected final collision error, got %v", err)
	}
	if len(store.circles) != 0 {
		t.Fatalf("collision retries must not persist a circle")
	}
}
