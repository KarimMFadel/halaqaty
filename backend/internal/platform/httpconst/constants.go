package httpconst

import (
	"mime"
	"strings"
)

const (
	HeaderAuthorization = "Authorization"
	HeaderContentType   = "Content-Type"
	HeaderForwardedFor  = "X-Forwarded-For"
	HeaderSessionID     = "X-Halaqaty-Session-ID"
	HeaderRequestID     = "X-Request-ID"
	// HeaderIdempotencyKey is the required retry-identity header of the
	// idempotent chat send contracts (F-003, F-004).
	HeaderIdempotencyKey = "Idempotency-Key"
)

const (
	AuthSchemeBearer = "Bearer"
)

const (
	ContentTypeApplicationJSON = "application/json"
)

// IsJSONContentType reports whether the header value is application/json.
func IsJSONContentType(contentTypeHeader string) bool {
	trimmedContentType := strings.TrimSpace(contentTypeHeader)
	if trimmedContentType == "" {
		return false
	}

	mediaType, _, parseError := mime.ParseMediaType(trimmedContentType)
	if parseError != nil {
		return false
	}

	return mediaType == ContentTypeApplicationJSON
}
