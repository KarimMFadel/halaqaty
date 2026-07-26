package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"halaqaty/backend/internal/auth"
	"halaqaty/backend/internal/platform/httpconst"
)

type authContextKey string

const principalContextKey authContextKey = "auth-principal"

// AuthPrincipal is the authenticated request identity.
type AuthPrincipal struct {
	UserID      string
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

// Require enforces auth and valid session.
func (m *AuthMiddleware) Require(next http.Handler) http.Handler {
	if m == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.verifier == nil || m.sessionService == nil || m.sessionRepo == nil {
			http.Error(w, httpconst.ErrorMessageAuthMiddlewareNotConfigured, http.StatusInternalServerError)
			return
		}

		bearerToken, ok := extractBearerToken(r.Header.Get(httpconst.HeaderAuthorization))
		if !ok {
			http.Error(w, httpconst.ErrorMessageMissingOrInvalidBearerToken, http.StatusUnauthorized)
			return
		}

		decoded, err := m.verifier.Verify(r.Context(), bearerToken)
		if err != nil {
			http.Error(w, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}

		sessionID := strings.TrimSpace(r.Header.Get(httpconst.HeaderSessionID))
		if sessionID == "" {
			http.Error(w, httpconst.ErrorMessageMissingSessionID, http.StatusUnauthorized)
			return
		}

		session, err := m.sessionRepo.GetByID(r.Context(), sessionID)
		if err != nil {
			http.Error(w, httpconst.ErrorMessageInvalidSession, http.StatusUnauthorized)
			return
		}
		localUserID, err := m.sessionRepo.GetLocalUserIDByFirebaseUID(r.Context(), decoded.UID)
		if err != nil {
			http.Error(w, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}
		if !strings.EqualFold(session.UserID, localUserID) {
			http.Error(w, httpconst.ErrorMessageSessionUserMismatch, http.StatusUnauthorized)
			return
		}
		if m.sessionService.IsExpired(session) {
			http.Error(w, httpconst.ErrorMessageSessionExpired, http.StatusUnauthorized)
			return
		}

		m.sessionService.Touch(&session)
		if err := m.sessionRepo.Touch(r.Context(), session.ID, session.LastActivityAt); err != nil {
			http.Error(w, httpconst.ErrorMessageInvalidSession, http.StatusUnauthorized)
			return
		}

		principal := AuthPrincipal{
			UserID:      localUserID,
			FirebaseUID: decoded.UID,
			Email:       decoded.Email,
			Claims:      decoded.Claims,
		}
		ctx := context.WithValue(r.Context(), principalContextKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
