package profile

import (
	"errors"
	"net/http"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// Handler exposes HTTP endpoints for profile flows.
type Handler struct {
	service *Service
}

// NewHandler constructs a profile HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetMe handles GET /auth/me.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageProfileHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	profile, err := h.service.GetMe(r.Context(), principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, profile)
}

// UpdateMe handles PUT /auth/me.
func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageProfileHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req UpdateProfileRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	updated, err := h.service.UpdateMe(r.Context(), principal.UserID, req)
	if err != nil {
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, validationErr.Fields)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, updated)
}
