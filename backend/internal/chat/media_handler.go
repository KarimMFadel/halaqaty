package chat

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// MediaRenewalService is the transport-facing seam the media handler depends
// on; *UploadService satisfies it in production, and the REST contract tests
// substitute deterministic fakes so the HTTP surface does not require
// PostgreSQL or MinIO.
type MediaRenewalService interface {
	// RenewMediaURL reauthorizes the viewer and returns a fresh presigned
	// URL for one attached media message.
	RenewMediaURL(ctx context.Context, viewerID, messageID uuid.UUID) (MediaAccess, error)
}

// MediaHandler exposes the F-004 US3 media-link renewal REST operation.
// Handlers decode, delegate to the service seam, and project responses; they
// contain no SQL and no business logic, and every authorization denial is
// the same non-enumerating envelope.
type MediaHandler struct {
	service MediaRenewalService
}

// NewMediaHandler constructs the media renewal handler over the service
// seam. A nil service reports internal server errors, matching the
// unconfigured handler convention of the other route families.
func NewMediaHandler(service MediaRenewalService) *MediaHandler {
	return &MediaHandler{service: service}
}

// mediaAccessResponse is the canonical MediaAccess projection: a fresh
// versionless presigned URL and its expiry, nothing else.
type mediaAccessResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// RenewMessageMediaURL implements the renewMessageMediaUrl contract
// operation: one freshly reauthorized seven-day presigned URL for an
// attached media message. Missing, deleted, text, unattached, and
// unauthorized messages all produce the same non-enumerating denial.
func (h *MediaHandler) RenewMessageMediaURL(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	viewerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid,
		})
		return
	}

	access, err := h.service.RenewMediaURL(r.Context(), viewerID, messageID)
	if err != nil {
		writeMediaRenewalError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, mediaAccessResponse{
		URL:       access.URL.String(),
		ExpiresAt: access.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

// writeMediaRenewalError maps renewal service errors onto the contract's
// declared responses: every visibility denial — missing, deleted, text,
// unattached, unauthorized, or eligibility-lost — is the same
// non-enumerating 403, and anything else is an internal error that must not
// leak details.
func writeMediaRenewalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMessageNotVisible), errors.Is(err, ErrDMNotEligible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
