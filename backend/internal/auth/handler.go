package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

const (
	displayNameMinLen        = 2
	displayNameMaxLen        = 100
	deviceNameMaxLen         = 100
	defaultPreferredLanguage = "ar"
)

// supportedPreferredLanguages mirrors the contract enum for preferred_language.
var supportedPreferredLanguages = map[string]struct{}{"ar": {}, "en": {}}

// Handler exposes HTTP endpoints for authentication flows.
type Handler struct {
	service  *Service
	deletion *AccountDeletionService
	logger   *slog.Logger
}

// NewHandler constructs an auth handler bound to the application service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// SetAccountDeletionService wires the privacy-sensitive account closure flow.
func (h *Handler) SetAccountDeletionService(service *AccountDeletionService) {
	h.deletion = service
}

// SetLogger uses the API's structured logger for deletion stage events.
func (h *Handler) SetLogger(logger *slog.Logger) { h.logger = logger }

// DeleteMe handles DELETE /auth/me. The caller must explicitly confirm.
func (h *Handler) DeleteMe(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.deletion == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	if !req.Confirm {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"confirm": "must be true"})
		return
	}
	result, err := h.deletion.Delete(r.Context(), principal.UserID, principal.AuthTime)
	if err != nil {
		switch {
		case errors.Is(err, ErrRecentReauthentication):
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		case errors.Is(err, ErrAccountIneligible), errors.Is(err, ErrAccountActiveSession):
			phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageAccountDeletionBlocked, http.StatusConflict)
		case errors.Is(err, ErrAccountDeleted):
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		default:
			phttp.WriteError(w, httpconst.ErrorCodeServiceUnavailable, httpconst.ErrorMessageServiceUnavailable, http.StatusServiceUnavailable)
		}
		return
	}
	if result == AccountDeletionPending {
		logger := h.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(r.Context(), "account_deletion_pending", "request_id", phttp.RequestIDFromContext(r.Context()), "stage", "firebase_identity_removal")
		phttp.WriteJSON(w, http.StatusAccepted, map[string]string{"status": string(result)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Register handles POST /auth/register. It runs behind RequireVerifiedFirebase,
// so identity comes only from the verified bearer principal. First-time
// provisioning answers 201; an idempotent replay of the same Firebase identity
// answers 409 with a fresh BackendSessionResponse body.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := CurrentPrincipal(r.Context())
	if !ok || principal.FirebaseUID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req RegisterRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldDisplayName: httpconst.ErrorMessageDisplayNameRequired,
		})
		return
	}
	if length := utf8.RuneCountInString(displayName); length < displayNameMinLen || length > displayNameMaxLen {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldDisplayName: httpconst.ErrorMessageDisplayNameInvalid,
		})
		return
	}

	language := strings.TrimSpace(req.PreferredLanguage)
	if language == "" {
		language = defaultPreferredLanguage
	}
	if _, supported := supportedPreferredLanguages[language]; !supported {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldPreferredLanguage: httpconst.ErrorMessagePreferredLanguageInvalid,
		})
		return
	}

	result, err := h.service.Register(r.Context(), RegisterInput{
		FirebaseUID:       principal.FirebaseUID,
		Email:             principal.Email,
		DisplayName:       displayName,
		PreferredLanguage: language,
	})
	if err != nil {
		if errors.Is(err, ErrAccountDeleted) {
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}
		if errors.Is(err, ErrDuplicateEmail) {
			phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageEmailAlreadyRegistered, http.StatusConflict)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusConflict
	}
	phttp.WriteJSON(w, status, result.Response)
}

// CreateSession handles POST /auth/sessions behind RequireBearer; the
// principal already carries the resolved local user ID.
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var req CreateBackendSessionRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}

	var deviceName *string
	if trimmed := strings.TrimSpace(req.DeviceName); trimmed != "" {
		if utf8.RuneCountInString(trimmed) > deviceNameMaxLen {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				httpconst.FieldDeviceName: httpconst.ErrorMessageDeviceNameTooLong,
			})
			return
		}
		deviceName = &trimmed
	}

	response, err := h.service.CreateSession(r.Context(), principal.UserID, deviceName)
	if err != nil {
		if errors.Is(err, ErrAccountDeleted) {
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, response)
}

// Logout handles POST /auth/logout behind the full Require middleware, which
// has already validated and touched the session. It revokes only the session
// identified by X-Halaqaty-Session-ID.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		return
	}

	principal, ok := CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	sessionID := strings.TrimSpace(r.Header.Get(httpconst.HeaderSessionID))
	if sessionID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeSessionMissing, httpconst.ErrorMessageMissingSessionID, http.StatusUnauthorized)
		return
	}

	if err := h.service.Logout(r.Context(), principal.UserID, sessionID); err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
