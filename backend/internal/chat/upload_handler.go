package chat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// ChatUploadMaxBodyBytes is the multipart body allowance on the F-004 upload
// routes (FR-022): 21 MB envelopes the largest 20 MB voice note with
// multipart overhead. The router applies it in place of the 1 MiB global
// default; a nested http.MaxBytesReader inside the handler could not raise
// the outer cap, so the allowance is a router-chain concern.
const ChatUploadMaxBodyBytes int64 = 21 << 20

// UploadStagingService is the transport-facing seam the upload handler
// depends on; *UploadService satisfies it in production, and the REST
// contract tests substitute deterministic fakes so the HTTP surface does not
// require PostgreSQL or MinIO.
type UploadStagingService interface {
	// Stage validates and stores one private chat attachment bound to the
	// uploader and exactly one authorized conversation context.
	Stage(ctx context.Context, in StageUploadInput) (StagedUpload, error)
}

// UploadHandler exposes the F-004 US3 chat upload REST operations. Handlers
// decode multipart requests, delegate to the service seam, and project
// responses; they contain no SQL and no business logic. Filenames are
// transported to the service raw and sanitized there, and never appear in
// any response.
type UploadHandler struct {
	service UploadStagingService
}

// NewUploadHandler constructs the chat upload handler over the service seam.
// A nil service reports internal server errors, matching the unconfigured
// handler convention of the other route families.
func NewUploadHandler(service UploadStagingService) *UploadHandler {
	return &UploadHandler{service: service}
}

// UploadVoice implements the uploadVoice contract operation: one staged
// OGG/MPEG/MP4/WebM voice note of at most 20 MB and 300 declared seconds.
func (h *UploadHandler) UploadVoice(w http.ResponseWriter, r *http.Request) {
	h.stage(w, r, MessageTypeVoice)
}

// UploadImage implements the uploadImage contract operation: one staged
// JPEG/PNG image of at most 5 MB.
func (h *UploadHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	h.stage(w, r, MessageTypeImage)
}

// UploadFile implements the uploadFile contract operation: one staged PDF of
// at most 10 MB.
func (h *UploadHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	h.stage(w, r, MessageTypeFile)
}

// uploadResponse is the canonical UploadResponse projection: the legacy
// object_key compatibility field, the seven-day presigned preview URL, and
// the staged identity. It never carries filenames or uploader metadata.
type uploadResponse struct {
	ObjectKey    string `json:"object_key"`
	URL          string `json:"url"`
	UploadID     string `json:"upload_id"`
	URLExpiresAt string `json:"url_expires_at"`
}

// stage decodes one multipart upload of the given media family, delegates to
// the service seam, and writes the documented response. The 21 MB body cap
// is applied by the router chain; reads that exceed it surface here as
// *http.MaxBytesError and map to the contract's 413.
func (h *UploadHandler) stage(w http.ResponseWriter, r *http.Request, mediaType MessageType) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	uploaderID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}

	fileName, data, ok := readUploadFile(w, r)
	if !ok {
		return
	}

	target, ok := parseUploadTarget(w, r)
	if !ok {
		return
	}
	duration, ok := parseUploadDuration(w, r)
	if !ok {
		return
	}

	staged, err := h.service.Stage(r.Context(), StageUploadInput{
		UploaderID:       uploaderID,
		Target:           target,
		MediaType:        mediaType,
		Data:             data,
		DeclaredFileName: fileName,
		DurationSeconds:  duration,
	})
	if err != nil {
		writeUploadError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, uploadResponse{
		ObjectKey:    staged.ObjectKey,
		URL:          staged.URL.String(),
		UploadID:     staged.UploadID.String(),
		URLExpiresAt: staged.URLExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

// readUploadFile reads the required multipart file part together with its
// declared filename. Bodies over the router's 21 MB route cap surface as
// *http.MaxBytesError and map to the contract's 413; the error response is
// already written when ok is false.
func readUploadFile(w http.ResponseWriter, r *http.Request) (fileName string, data []byte, ok bool) {
	file, header, err := r.FormFile(httpconst.FieldFile)
	if err != nil {
		writeUploadReadError(w, err)
		return "", nil, false
	}
	defer func() { _ = file.Close() }()
	data, err = io.ReadAll(file)
	if err != nil {
		writeUploadReadError(w, err)
		return "", nil, false
	}
	return header.Filename, data, true
}

// parseUploadTarget reads the mutually exclusive circle_id/dm_peer_id form
// values. Malformed UUIDs are rejected as 400 field-level validation errors;
// the exactly-one-target rule itself is service-side (FR-023). The error
// response is already written when ok is false.
func parseUploadTarget(w http.ResponseWriter, r *http.Request) (UploadTarget, bool) {
	var target UploadTarget
	if raw := r.FormValue(httpconst.FieldCircleID); raw != "" {
		circleID, err := uuid.Parse(raw)
		if err != nil {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid,
			})
			return target, false
		}
		target.CircleID = &circleID
	}
	if raw := r.FormValue(httpconst.FieldDMPeerID); raw != "" {
		dmPeerID, err := uuid.Parse(raw)
		if err != nil {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				httpconst.FieldDMPeerID: httpconst.ErrorMessageChatDMPeerIDInvalid,
			})
			return target, false
		}
		target.DMPeerID = &dmPeerID
	}
	if (target.CircleID == nil) == (target.DMPeerID == nil) {
		writeFieldsUnprocessable(w, map[string]string{
			httpconst.FieldCircleID: httpconst.ErrorMessageChatUploadTargetInvalid,
			httpconst.FieldDMPeerID: httpconst.ErrorMessageChatUploadTargetInvalid,
		})
		return target, false
	}
	return target, true
}

// parseUploadDuration reads the optional client-declared voice duration in
// seconds; the 1-300 range for voice is service-side. The error response is
// already written when ok is false.
func parseUploadDuration(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.FormValue(httpconst.FieldDurationSeconds)
	if raw == "" {
		return 0, true
	}
	duration, err := strconv.Atoi(raw)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldDurationSeconds: httpconst.ErrorMessageChatDurationMalformed,
		})
		return 0, false
	}
	return duration, true
}

// writeUploadReadError maps multipart body-read failures onto the contract:
// a body over the 21 MB route cap is 413, anything else is a malformed
// request reported as a field-level 400 without internal detail.
func writeUploadReadError(w http.ResponseWriter, err error) {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		phttp.WriteError(w, httpconst.ErrorCodeUploadTooLarge, httpconst.ErrorMessageUploadTooLarge, http.StatusRequestEntityTooLarge)
		return
	}
	phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
		httpconst.FieldFile: httpconst.ErrorMessageChatFileRequired,
	})
}

// writeUploadError maps staging service errors onto the contract's declared
// responses: unsupported detected media is 415, an oversized attachment or
// body is 413, a missing or over-limit voice duration and an invalid target
// combination are 422, a non-visible circle target (non-member or unknown,
// indistinguishable) and an ineligible DM pair are the same non-enumerating
// 403, an exhausted rolling-hour budget is 429, and anything else is an
// internal error that must not leak details.
func writeUploadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMalformedMedia):
		writeFieldsUnprocessable(w, map[string]string{httpconst.FieldFile: httpconst.ErrorMessageChatMalformedMedia})
	case errors.Is(err, ErrUnsupportedMIME):
		phttp.WriteError(w, httpconst.ErrorCodeUnsupportedMediaType, httpconst.ErrorMessageUnsupportedMediaType, http.StatusUnsupportedMediaType)
	case errors.Is(err, ErrUploadTooLarge):
		phttp.WriteError(w, httpconst.ErrorCodeUploadTooLarge, httpconst.ErrorMessageUploadTooLarge, http.StatusRequestEntityTooLarge)
	case errors.Is(err, ErrInvalidDuration):
		writeFieldsUnprocessable(w, map[string]string{
			httpconst.FieldDurationSeconds: httpconst.ErrorMessageChatDurationInvalid,
		})
	case errors.Is(err, ErrInvalidContext):
		writeFieldsUnprocessable(w, map[string]string{
			httpconst.FieldCircleID: httpconst.ErrorMessageChatUploadTargetInvalid,
			httpconst.FieldDMPeerID: httpconst.ErrorMessageChatUploadTargetInvalid,
		})
	case errors.Is(err, ErrCircleNotVisible), errors.Is(err, ErrCircleArchived), errors.Is(err, ErrDMNotEligible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrUploadRateExceeded):
		phttp.WriteError(w, httpconst.ErrorCodeRateLimitExceeded, httpconst.ErrorMessageRateLimitExceeded, http.StatusTooManyRequests)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}

// writeFieldsUnprocessable writes one 422 validation envelope with
// field-level details, the multi-field sibling of writeFieldUnprocessable.
func writeFieldsUnprocessable(w http.ResponseWriter, fields map[string]string) {
	phttp.WriteJSON(w, http.StatusUnprocessableEntity, phttp.ErrorEnvelope{
		Error: phttp.ErrorBody{
			Code:    httpconst.ErrorCodeValidationFailed,
			Message: httpconst.ErrorMessageValidationFailed,
			Fields:  fields,
		},
	})
}
