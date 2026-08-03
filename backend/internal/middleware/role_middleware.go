package middleware

import (
	"context"
	"net/http"
	"strings"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// CircleMembershipRepository resolves role by circle and user.
type CircleMembershipRepository interface {
	RoleForUserInCircle(ctx context.Context, circleID string, userID string) (string, error)
}

// RoleMiddleware enforces per-circle authorization.
type RoleMiddleware struct {
	repo CircleMembershipRepository
}

// NewRoleMiddleware creates role-check middleware.
func NewRoleMiddleware(repo CircleMembershipRepository) *RoleMiddleware {
	return &RoleMiddleware{repo: repo}
}

// RequireAny restricts access to users holding one of the allowed roles.
func (m *RoleMiddleware) RequireAny(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if m == nil {
			return next
		}

		normalizedAllowedRoles := normalizeRoles(allowedRoles)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if m.repo == nil {
				phttp.WriteError(
					w,
					httpconst.ErrorCodeInternalServerError,
					httpconst.ErrorMessageRoleMiddlewareNotConfigured,
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

			circleID := strings.TrimSpace(r.PathValue("circleId"))
			if circleID == "" {
				circleID = strings.TrimSpace(r.URL.Query().Get("circleId"))
			}
			if circleID == "" {
				phttp.WriteValidationError(
					w,
					httpconst.ErrorMessageMissingCircleID,
					map[string]string{httpconst.FieldCircleID: "required"},
				)
				return
			}

			role, err := m.repo.RoleForUserInCircle(r.Context(), circleID, principal.UserID)
			if err != nil {
				phttp.WriteError(
					w,
					httpconst.ErrorCodeForbidden,
					httpconst.ErrorMessageForbidden,
					http.StatusForbidden,
				)
				return
			}
			if !containsRole(normalizedAllowedRoles, normalizeRole(role)) {
				phttp.WriteError(
					w,
					httpconst.ErrorCodeForbidden,
					httpconst.ErrorMessageForbidden,
					http.StatusForbidden,
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func normalizeRoles(roles []string) []string {
	normalized := make([]string, 0, len(roles))
	for _, role := range roles {
		normalized = append(normalized, normalizeRole(role))
	}
	return normalized
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func containsRole(allowedRoles []string, currentRole string) bool {
	for _, role := range allowedRoles {
		if role == currentRole {
			return true
		}
	}
	return false
}
