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

// UpdateCircle handles PUT /circles/{circleId}.
func (h *Handler) UpdateCircle(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if h == nil || h.service == nil || !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	var req UpdateCircleRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	circle, err := h.service.UpdateCircle(r.Context(), principal.UserID, r.PathValue("circleId"), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, circle)
}

// SearchUsers handles GET /users/search.
func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError,
			httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized,
			httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	users, err := h.service.SearchUsers(r.Context(), r.URL.Query().Get(httpconst.FieldQuery))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, UserSearchResponse{Data: users})
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

// JoinPublicCircle handles POST /circles/{circleId}/join.
func (h *Handler) JoinPublicCircle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	circle, err := h.service.JoinPublicCircle(r.Context(), principal.UserID, r.PathValue("circleId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, circle)
}

// DiscoverPublicCircles handles GET /circles/discover.
func (h *Handler) DiscoverPublicCircles(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	result, err := h.service.DiscoverPublicCircles(r.Context(), r.URL.Query().Get(httpconst.FieldDiscoverQuery), r.URL.Query().Get(httpconst.FieldCursor))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, result)
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

// RefreshInviteCode handles POST /circles/{circleId}/invite-code/refresh.
func (h *Handler) RefreshInviteCode(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if h == nil || h.service == nil || !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	code, err := h.service.RefreshInviteCode(r.Context(), principal.UserID, r.PathValue("circleId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, InviteResponse{InviteCode: code, InviteLink: inviteLinkBase + code})
}

// RemoveMember handles DELETE /circles/{circleId}/members/{userId}.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if h == nil || h.service == nil || !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	if err := h.service.RemoveMember(r.Context(), principal.UserID, r.PathValue("circleId"), r.PathValue("userId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ArchiveCircle handles DELETE /circles/{circleId} as soft retirement only.
func (h *Handler) ArchiveCircle(w http.ResponseWriter, r *http.Request) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if h == nil || h.service == nil || !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	if err := h.service.ArchiveCircle(r.Context(), principal.UserID, r.PathValue("circleId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetCircle handles GET /circles/{circleId}.
func (h *Handler) GetCircle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	circle, err := h.service.GetCircle(r.Context(), principal.UserID, r.PathValue("circleId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, circle)
}

// ListMembers handles GET /circles/{circleId}/members.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	members, err := h.service.ListMembers(r.Context(), principal.UserID, r.PathValue("circleId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, MemberListResponse{Data: members})
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
	case errors.Is(err, ErrCirclePrivate):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCirclePrivate, http.StatusConflict)
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
