package http

import (
	"encoding/json"
	"net/http"

	"halaqaty/backend/internal/platform/httpconst"
)

// ErrorEnvelope is the standard error response shape.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contains the error code, message, and optional field map.
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// WriteError writes a JSON error envelope and status code.
func WriteError(w http.ResponseWriter, code string, message string, status int) {
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorBody{Code: code, Message: message},
	})
}

// WriteValidationError writes a validation-style error envelope with field-level details.
func WriteValidationError(w http.ResponseWriter, message string, fields map[string]string) {
	if fields == nil {
		fields = map[string]string{}
	}
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorBody{
			Code:    httpconst.ErrorCodeValidationFailed,
			Message: message,
			Fields:  fields,
		},
	})
}
