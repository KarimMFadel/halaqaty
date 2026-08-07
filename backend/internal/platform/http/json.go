package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// DecodeJSONBody decodes an optional JSON body and rejects unknown fields.
func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
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
		WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBody: message,
		})
		return false
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBody: httpconst.ErrorMessageRequestBodyInvalid,
		})
		return false
	}
	return true
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
