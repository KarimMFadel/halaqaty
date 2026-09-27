package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
)

// mockFirebaseClient stubs firebaseTokenClient.
type mockFirebaseClient struct {
	token        *firebaseauth.Token
	revokedToken *firebaseauth.Token
	err          error
	revokedErr   error
	revokedCalls int
}

func (m *mockFirebaseClient) VerifyIDToken(_ context.Context, _ string) (*firebaseauth.Token, error) {
	return m.token, m.err
}

func (m *mockFirebaseClient) VerifyIDTokenAndCheckRevoked(_ context.Context, _ string) (*firebaseauth.Token, error) {
	m.revokedCalls++
	return m.revokedToken, m.revokedErr
}

func TestFirebaseVerifier_VerifyAndCheckRevokedUsesRevocationAwareSDKMethod(t *testing.T) {
	client := &mockFirebaseClient{revokedToken: &firebaseauth.Token{
		UID: "firebase-uid-revocation", AuthTime: time.Now().UTC().Add(-time.Minute).Unix(),
		Expires: time.Now().UTC().Add(time.Hour).Unix(), Claims: map[string]any{"email": "user@example.com"},
	}}
	verifier := NewFirebaseVerifier(client)
	decoded, err := verifier.VerifyAndCheckRevoked(context.Background(), "signed-token")
	if err != nil {
		t.Fatalf("VerifyAndCheckRevoked: %v", err)
	}
	if client.revokedCalls != 1 || decoded.UID != "firebase-uid-revocation" || decoded.AuthTime.IsZero() {
		t.Fatalf("revocation verification: calls=%d token=%+v", client.revokedCalls, decoded)
	}
}

func TestFirebaseVerifier_Verify(t *testing.T) {
	future := time.Now().UTC().Add(time.Hour).Unix()
	authTime := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Second)

	t.Run("returns DecodedToken on valid token", func(t *testing.T) {
		client := &mockFirebaseClient{
			token: &firebaseauth.Token{
				UID:      "firebase-uid-1",
				AuthTime: authTime.Unix(),
				IssuedAt: time.Now().UTC().Add(-time.Minute).Unix(),
				Expires:  future,
				Claims: map[string]any{
					"email": "user@example.com",
				},
			},
		}
		v := NewFirebaseVerifier(client)
		decoded, err := v.Verify(context.Background(), "some-token")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if decoded.UID != "firebase-uid-1" {
			t.Fatalf("UID: got %q, want %q", decoded.UID, "firebase-uid-1")
		}
		if decoded.Email != "user@example.com" {
			t.Fatalf("Email: got %q, want %q", decoded.Email, "user@example.com")
		}
		if decoded.ExpiresAt.IsZero() {
			t.Fatal("expected non-zero ExpiresAt")
		}
		if !decoded.AuthTime.Equal(authTime) {
			t.Fatalf("AuthTime: got %v, want %v", decoded.AuthTime, authTime)
		}
	})

	t.Run("returns error on client error", func(t *testing.T) {
		client := &mockFirebaseClient{err: errors.New("firebase: invalid token")}
		v := NewFirebaseVerifier(client)
		_, err := v.Verify(context.Background(), "bad-token")
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("returns ErrEmptyToken on empty bearer", func(t *testing.T) {
		client := &mockFirebaseClient{}
		v := NewFirebaseVerifier(client)
		_, err := v.Verify(context.Background(), "")
		if !errors.Is(err, ErrEmptyToken) {
			t.Fatalf("expected ErrEmptyToken, got %v", err)
		}
	})

	t.Run("returns ErrExpiredToken on already-expired token", func(t *testing.T) {
		past := time.Now().UTC().Add(-time.Hour).Unix()
		client := &mockFirebaseClient{
			token: &firebaseauth.Token{
				UID:     "firebase-uid-2",
				Expires: past,
				Claims:  map[string]any{},
			},
		}
		v := NewFirebaseVerifier(client)
		_, err := v.Verify(context.Background(), "expired-token")
		if !errors.Is(err, ErrExpiredToken) {
			t.Fatalf("expected ErrExpiredToken, got %v", err)
		}
	})
}
