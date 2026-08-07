package rbac

import (
	"errors"
	"net/http"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// Handler exposes HTTP endpoints for circle role-management flows.
type Handler struct {
	service *Service
}

// NewHandler constructs a circle RBAC HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// CreateCircle handles POST /circles.
func (h *Handler) CreateCircle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req CreateCircleRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	circle, err := h.service.CreateCircle(r.Context(), principal.UserID, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, circle)
}

// JoinCircle handles POST /circles/join.
func (h *Handler) JoinCircle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req JoinCircleRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	circle, err := h.service.JoinCircle(r.Context(), principal.UserID, req.InviteCode)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, circle)
}

// AssignRole handles PUT /circles/{circleId}/members/{userId}/role.
func (h *Handler) AssignRole(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req AssignCircleRoleRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	assignment, err := h.service.AssignRole(
		r.Context(),
		principal.UserID,
		r.PathValue("circleId"),
		r.PathValue("userId"),
		req.Role,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, assignment)
}

// writeServiceError maps service errors onto the standard error envelope.
func writeServiceError(w http.ResponseWriter, err error) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, validationErr.Fields)
		return
	}

	switch {
	case errors.Is(err, ErrCircleNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageCircleNotFound, http.StatusNotFound)
	case errors.Is(err, ErrMemberNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageCircleMemberNotFound, http.StatusNotFound)
	case errors.Is(err, ErrAlreadyMember):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleAlreadyMember, http.StatusConflict)
	case errors.Is(err, ErrCircleArchived):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleArchived, http.StatusConflict)
	case errors.Is(err, ErrCircleFull):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleFull, http.StatusConflict)
	case errors.Is(err, ErrCircleLimit):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleLimit, http.StatusConflict)
	case errors.Is(err, ErrSelfRoleChange):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageSelfRoleChangeForbidden, http.StatusForbidden)
	case errors.Is(err, ErrFinalTeacher):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageFinalTeacherRequired, http.StatusForbidden)
	case errors.Is(err, ErrForbidden):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
