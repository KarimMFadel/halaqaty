package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
)

var (
	// ErrEmptyToken indicates a missing bearer token.
	ErrEmptyToken = errors.New("empty bearer token")
	// ErrExpiredToken indicates an expired Firebase ID token.
	ErrExpiredToken = errors.New("expired bearer token")
)

// DecodedToken is a normalized representation used by middleware/services.
type DecodedToken struct {
	UID       string
	Email     string
	Claims    map[string]any
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// TokenVerifier verifies identity tokens.
type TokenVerifier interface {
	Verify(ctx context.Context, bearerToken string) (*DecodedToken, error)
}

type firebaseTokenClient interface {
	VerifyIDToken(ctx context.Context, idToken string) (*firebaseauth.Token, error)
}

// FirebaseVerifier verifies Firebase-issued ID tokens for protected endpoints.
type FirebaseVerifier struct {
	client firebaseTokenClient
	nowFn  func() time.Time
}

// NewFirebaseVerifier builds a verifier with defaults.
func NewFirebaseVerifier(client firebaseTokenClient) *FirebaseVerifier {
	return &FirebaseVerifier{
		client: client,
		nowFn:  time.Now,
	}
}

// Verify validates and decodes a bearer token.
func (v *FirebaseVerifier) Verify(ctx context.Context, bearerToken string) (*DecodedToken, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return nil, ErrEmptyToken
	}

	raw, err := v.client.VerifyIDToken(ctx, token)
	if err != nil {
		return nil, err
	}

	issuedAt := time.Unix(raw.IssuedAt, 0).UTC()
	expiresAt := time.Unix(raw.Expires, 0).UTC()
	if !expiresAt.After(v.nowFn().UTC()) {
		return nil, ErrExpiredToken
	}

	email, _ := raw.Claims["email"].(string)

	return &DecodedToken{
		UID:       raw.UID,
		Email:     email,
		Claims:    raw.Claims,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}
