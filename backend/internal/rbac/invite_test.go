package rbac

import (
	"context"
	"errors"
	"strings"
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

// TestGenerateInviteCode_TwelveSymbols pins the brute-force resistance
// requirement: 32-symbol alphabet × 12 random positions ≈ 2^60 keyspace.
func TestGenerateInviteCode_TwelveSymbols(t *testing.T) {
	for range 100 {
		code, err := generateInviteCode()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(code, inviteCodePrefix) {
			t.Fatalf("code %q missing prefix %q", code, inviteCodePrefix)
		}
		symbols := code[len(inviteCodePrefix):]
		if len(symbols) != 12 {
			t.Fatalf("symbol count: got %d (%q), want 12", len(symbols), code)
		}
		for _, char := range symbols {
			if !strings.ContainsRune(inviteCodeAlphabet, char) {
				t.Fatalf("symbol %q in %q outside approved alphabet", char, code)
			}
		}
	}
}

// TestIsInviteCode_AcceptsLegacyAndCurrentLengths keeps the join path
// compatible with pre-existing 4-symbol codes (circles.invite_code VARCHAR(20),
// exact-match lookup) while enforcing the 12-symbol generation format.
func TestIsInviteCode_AcceptsLegacyAndCurrentLengths(t *testing.T) {
	cases := []struct {
		name string
		code string
		want bool
	}{
		{"legacy four symbols", "HLQ-7X2K", true},
		{"current twelve symbols", "HLQ-ABCDEFGHJKLM", true},
		{"eleven symbols rejected", "HLQ-ABCDEFGHJK", false},
		{"thirteen symbols rejected", "HLQ-ABCDEFGHIJKLM", false},
	}
	for _, tc := range cases {
		if got := isInviteCode(tc.code); got != tc.want {
			t.Errorf("%s: isInviteCode(%q) = %v, want %v", tc.name, tc.code, got, tc.want)
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
