package auth

import (
	"testing"
	"time"
)

func TestSession_IsExpired(t *testing.T) {
	now := time.Now().UTC()

	t.Run("returns true when ExpiresAt is in the past", func(t *testing.T) {
		s := Session{ExpiresAt: now.Add(-time.Second)}
		if !s.IsExpired(now) {
			t.Fatal("expected IsExpired to return true for past ExpiresAt")
		}
	})

	t.Run("returns false when ExpiresAt is in the future", func(t *testing.T) {
		s := Session{ExpiresAt: now.Add(time.Hour)}
		if s.IsExpired(now) {
			t.Fatal("expected IsExpired to return false for future ExpiresAt")
		}
	})

	t.Run("returns false when ExpiresAt is zero", func(t *testing.T) {
		s := Session{}
		if s.IsExpired(now) {
			t.Fatal("expected IsExpired to return false for zero ExpiresAt")
		}
	})
}

func TestSessionService_IsExpired(t *testing.T) {
	now := time.Now().UTC()
	inactivityTimeout := 30 * time.Minute

	newSvc := func(nowFn func() time.Time) *SessionService {
		svc := NewSessionService(inactivityTimeout)
		svc.nowFn = nowFn
		return svc
	}

	t.Run("returns true for revoked session", func(t *testing.T) {
		svc := newSvc(func() time.Time { return now })
		revokedAt := now.Add(-time.Hour)
		s := Session{
			ExpiresAt:      now.Add(time.Hour),
			LastActivityAt: now.Add(-time.Minute),
			RevokedAt:      &revokedAt,
		}
		if !svc.IsExpired(s) {
			t.Fatal("expected IsExpired true for revoked session")
		}
	})

	t.Run("returns true for absolutely expired session", func(t *testing.T) {
		svc := newSvc(func() time.Time { return now })
		s := Session{
			ExpiresAt:      now.Add(-time.Second),
			LastActivityAt: now.Add(-time.Minute),
		}
		if !svc.IsExpired(s) {
			t.Fatal("expected IsExpired true for expired session")
		}
	})

	t.Run("returns true for inactive session beyond timeout", func(t *testing.T) {
		svc := newSvc(func() time.Time { return now })
		s := Session{
			ExpiresAt:      now.Add(time.Hour),
			LastActivityAt: now.Add(-(inactivityTimeout + time.Second)),
		}
		if !svc.IsExpired(s) {
			t.Fatal("expected IsExpired true for inactive session")
		}
	})

	t.Run("returns false for active valid session", func(t *testing.T) {
		svc := newSvc(func() time.Time { return now })
		s := Session{
			ExpiresAt:      now.Add(time.Hour),
			LastActivityAt: now.Add(-time.Minute),
		}
		if svc.IsExpired(s) {
			t.Fatal("expected IsExpired false for valid active session")
		}
	})
}

func TestSessionService_Touch(t *testing.T) {
	fixedTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := NewSessionService(30 * time.Minute)
	svc.nowFn = func() time.Time { return fixedTime }

	s := Session{LastActivityAt: fixedTime.Add(-time.Hour)}
	svc.Touch(&s)

	if !s.LastActivityAt.Equal(fixedTime) {
		t.Fatalf("Touch: got %v, want %v", s.LastActivityAt, fixedTime)
	}
}
