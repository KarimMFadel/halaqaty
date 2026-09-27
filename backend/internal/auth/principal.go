package auth

import (
	"context"
	"time"
)

type principalContextKey struct{}

// AuthPrincipal is the authenticated request identity resolved from the
// verified Firebase bearer token and, when present, the backend session.
type AuthPrincipal struct {
	UserID      string // local PostgreSQL UUID; empty before registration
	FirebaseUID string
	Email       string
	Claims      map[string]any
	AuthTime    time.Time
}

// WithPrincipal stores the authenticated principal in the request context.
func WithPrincipal(ctx context.Context, principal AuthPrincipal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// CurrentPrincipal returns the request principal from context, if available.
func CurrentPrincipal(ctx context.Context) (AuthPrincipal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(AuthPrincipal)
	return principal, ok
}
