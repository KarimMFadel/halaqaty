package chat

import (
	"context"
	"errors"
	"net/http"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/google/uuid"
)

// ModerationHandler exposes group-message deletion without returning content
// or audit details.
type ModerationHandler struct{ service ModerationService }

// NewModerationHandler constructs a moderation handler.
func NewModerationHandler(service ModerationService) *ModerationHandler {
	return &ModerationHandler{service: service}
}

// DeleteCircleMessage deletes one message under the caller's current authority.
func (h *ModerationHandler) DeleteCircleMessage(w http.ResponseWriter, r *http.Request) {
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	actor, err := uuid.Parse(principal.UserID)
	circle, circleErr := uuid.Parse(r.PathValue("circleId"))
	message, messageErr := uuid.Parse(r.PathValue("messageId"))
	if err != nil || circleErr != nil || messageErr != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if err := h.service.Delete(r.Context(), actor, circle, message); err != nil {
		status := http.StatusInternalServerError
		code := httpconst.ErrorCodeInternalServerError
		if errors.Is(err, ErrCircleArchived) || errors.Is(err, ErrDirectDeleteConflict) {
			status, code = http.StatusConflict, httpconst.ErrorCodeConflict
		}
		if errors.Is(err, ErrCircleNotVisible) || errors.Is(err, ErrDMNotEligible) {
			status, code = http.StatusForbidden, httpconst.ErrorCodeForbidden
		}
		if errors.Is(err, ErrMessageNotVisible) {
			status, code = http.StatusNotFound, httpconst.ErrorCodeNotFound
		}
		phttp.WriteError(w, code, httpconst.ErrorMessageForbidden, status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var _ interface {
	Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
} = (*ModerationServiceImpl)(nil)
