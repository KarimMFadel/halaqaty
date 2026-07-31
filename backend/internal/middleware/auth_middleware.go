package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"halaqaty/backend/internal/auth"
	phttp "halaqaty/backend/internal/platform/http"
	"halaqaty/backend/internal/platform/httpconst"
)

type authContextKey string

const principalContextKey authContextKey = "auth-principal"

// AuthPrincipal is the authenticated request identity.
type AuthPrincipal struct {
	UserID      string // local PostgreSQL UUID; empty before registration
	FirebaseUID string
	Email       string
	Claims      map[string]any
}

// CurrentPrincipal returns the request principal from context, if available.
func CurrentPrincipal(ctx context.Context) (AuthPrincipal, bool) {
	principal, ok := ctx.Value(principalContextKey).(AuthPrincipal)
	return principal, ok
}

// SessionRepository defines the session storage needed by auth middleware.
type SessionRepository interface {
	GetByID(ctx context.Context, sessionID string) (auth.Session, error)
	Touch(ctx context.Context, sessionID string, lastActivityAt time.Time) error
	GetLocalUserIDByFirebaseUID(ctx context.Context, firebaseUID string) (string, error)
}

// AuthMiddleware verifies bearer tokens and backend session state.
type AuthMiddleware struct {
	verifier       auth.TokenVerifier
	sessionService *auth.SessionService
	sessionRepo    SessionRepository
}

// NewAuthMiddleware creates middleware for protected routes.
func NewAuthMiddleware(
	verifier auth.TokenVerifier,
	sessionService *auth.SessionService,
	sessionRepo SessionRepository,
) *AuthMiddleware {
	return &AuthMiddleware{
		verifier:       verifier,
		sessionService: sessionService,
		sessionRepo:    sessionRepo,
	}
}

// RequireBearer validates the Firebase bearer token, resolves the existing local
// user, and sets the AuthPrincipal in the request context. It is used for
// endpoints that create backend sessions for an already-registered user and
// therefore cannot yet require a session ID.
func (m *AuthMiddleware) RequireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil || m.verifier == nil || m.sessionRepo == nil {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeInternalServerError,
				httpconst.ErrorMessageAuthMiddlewareNotConfigured,
				http.StatusInternalServerError,
			)
			return
		}

		principal, ok := m.authenticateBearer(r)
		if !ok {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeUnauthorized,
				httpconst.ErrorMessageUnauthorized,
				http.StatusUnauthorized,
			)
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireVerifiedFirebase validates only the Firebase bearer token and sets an
// AuthPrincipal with FirebaseUID, Email, and Claims. The local UserID is left
// empty because the user row is created by the downstream registration handler.
// It is used exclusively for the registration endpoint.
func (m *AuthMiddleware) RequireVerifiedFirebase(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil || m.verifier == nil {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeInternalServerError,
				httpconst.ErrorMessageAuthMiddlewareNotConfigured,
				http.StatusInternalServerError,
			)
			return
		}

		decoded, ok := m.verifyBearer(r)
		if !ok {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeUnauthorized,
				httpconst.ErrorMessageUnauthorized,
				http.StatusUnauthorized,
			)
			return
		}

		principal := AuthPrincipal{
			FirebaseUID: decoded.UID,
			Email:       decoded.Email,
			Claims:      decoded.Claims,
		}
		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Require enforces both a valid Firebase bearer token and a matching backend
// session identified by X-Halaqaty-Session-ID. It is used for all protected
// routes except registration and backend-session creation.
func (m *AuthMiddleware) Require(next http.Handler) http.Handler {
	return m.RequireBearer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.sessionService == nil {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeInternalServerError,
				httpconst.ErrorMessageAuthMiddlewareNotConfigured,
				http.StatusInternalServerError,
			)
			return
		}

		principal, ok := CurrentPrincipal(r.Context())
		if !ok {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeUnauthorized,
				httpconst.ErrorMessageUnauthorized,
				http.StatusUnauthorized,
			)
			return
		}

		sessionID := strings.TrimSpace(r.Header.Get(httpconst.HeaderSessionID))
		if sessionID == "" {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeSessionMissing,
				httpconst.ErrorMessageMissingSessionID,
				http.StatusUnauthorized,
			)
			return
		}

		session, err := m.sessionRepo.GetByID(r.Context(), sessionID)
		if err != nil {
			code := httpconst.ErrorCodeUnauthorized
			if errors.Is(err, auth.ErrSessionNotFound) {
				code = httpconst.ErrorCodeSessionNotFound
			}
			phttp.WriteError(
				w,
				code,
				httpconst.ErrorMessageInvalidSession,
				http.StatusUnauthorized,
			)
			return
		}

		if !strings.EqualFold(session.UserID, principal.UserID) {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeSessionUserMismatch,
				httpconst.ErrorMessageSessionUserMismatch,
				http.StatusUnauthorized,
			)
			return
		}

		if session.IsRevoked() {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeSessionRevoked,
				httpconst.ErrorMessageSessionRevoked,
				http.StatusUnauthorized,
			)
			return
		}

		if m.sessionService.IsExpired(session) {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeSessionExpired,
				httpconst.ErrorMessageSessionExpired,
				http.StatusUnauthorized,
			)
			return
		}

		m.sessionService.Touch(&session)
		if err := m.sessionRepo.Touch(r.Context(), session.ID, session.LastActivityAt); err != nil {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeUnauthorized,
				httpconst.ErrorMessageInvalidSession,
				http.StatusUnauthorized,
			)
			return
		}

		next.ServeHTTP(w, r)
	}))
}

func (m *AuthMiddleware) authenticateBearer(r *http.Request) (AuthPrincipal, bool) {
	decoded, ok := m.verifyBearer(r)
	if !ok {
		return AuthPrincipal{}, false
	}

	localUserID, err := m.sessionRepo.GetLocalUserIDByFirebaseUID(r.Context(), decoded.UID)
	if err != nil {
		return AuthPrincipal{}, false
	}

	return AuthPrincipal{
		UserID:      localUserID,
		FirebaseUID: decoded.UID,
		Email:       decoded.Email,
		Claims:      decoded.Claims,
	}, true
}

func (m *AuthMiddleware) verifyBearer(r *http.Request) (*auth.DecodedToken, bool) {
	bearerToken, ok := extractBearerToken(r.Header.Get(httpconst.HeaderAuthorization))
	if !ok {
		return nil, false
	}

	decoded, err := m.verifier.Verify(r.Context(), bearerToken)
	if err != nil {
		return nil, false
	}

	return decoded, true
}

func extractBearerToken(rawAuthHeader string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(rawAuthHeader), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], httpconst.AuthSchemeBearer) {
		return "", false
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}

	return token, true
}
