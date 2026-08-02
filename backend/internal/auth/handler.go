package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	phttp "halaqaty/backend/internal/platform/http"
	"halaqaty/backend/internal/platform/httpconst"
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
	service *Service
}

// NewHandler constructs an auth handler bound to the application service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
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
	if !decodeJSONBody(w, r, &req) {
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
	writeJSON(w, status, result.Response)
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
	if !decodeJSONBody(w, r, &req) {
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
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, response)
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

// decodeJSONBody decodes an optional JSON body into dst, rejecting unknown
// fields (e.g. a password field) and malformed payloads with a 400 validation
// envelope. An empty body is treated as a zero-value payload.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil {
		return true
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		message := httpconst.ErrorMessageRequestBodyInvalid
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			message = httpconst.ErrorMessageRequestBodyUnknownFields
		}
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBody: message,
		})
		return false
	}

	// Reject trailing non-whitespace JSON/data after the first value.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBody: httpconst.ErrorMessageRequestBodyInvalid,
		})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
