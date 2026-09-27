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
	// ErrRevocationCheckUnavailable means Firebase could not confirm token revocation.
	ErrRevocationCheckUnavailable = errors.New("Firebase token revocation check is unavailable")
)

// DecodedToken is a normalized representation used by middleware/services.
type DecodedToken struct {
	UID       string
	Email     string
	Claims    map[string]any
	AuthTime  time.Time
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// TokenVerifier verifies identity tokens.
type TokenVerifier interface {
	Verify(ctx context.Context, bearerToken string) (*DecodedToken, error)
}

// RevocationAwareTokenVerifier verifies tokens and checks Firebase revocation.
type RevocationAwareTokenVerifier interface {
	VerifyAndCheckRevoked(ctx context.Context, bearerToken string) (*DecodedToken, error)
}

type firebaseTokenClient interface {
	VerifyIDToken(ctx context.Context, idToken string) (*firebaseauth.Token, error)
}

type firebaseRevocationTokenClient interface {
	VerifyIDTokenAndCheckRevoked(ctx context.Context, idToken string) (*firebaseauth.Token, error)
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
	return v.decode(raw)
}

// VerifyAndCheckRevoked verifies the bearer and asks Firebase Admin to reject
// revoked tokens. Registration and session provisioning use this fail-closed path.
func (v *FirebaseVerifier) VerifyAndCheckRevoked(ctx context.Context, bearerToken string) (*DecodedToken, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return nil, ErrEmptyToken
	}
	client, ok := v.client.(firebaseRevocationTokenClient)
	if !ok {
		return nil, ErrRevocationCheckUnavailable
	}
	raw, err := client.VerifyIDTokenAndCheckRevoked(ctx, token)
	if err != nil {
		return nil, err
	}
	return v.decode(raw)
}

func (v *FirebaseVerifier) decode(raw *firebaseauth.Token) (*DecodedToken, error) {
	issuedAt := time.Unix(raw.IssuedAt, 0).UTC()
	expiresAt := time.Unix(raw.Expires, 0).UTC()
	if !expiresAt.After(v.nowFn().UTC()) {
		return nil, ErrExpiredToken
	}

	email, _ := raw.Claims["email"].(string)
	var authTime time.Time
	if raw.AuthTime > 0 {
		authTime = time.Unix(raw.AuthTime, 0).UTC()
	}

	return &DecodedToken{
		UID:       raw.UID,
		Email:     email,
		Claims:    raw.Claims,
		AuthTime:  authTime,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}
