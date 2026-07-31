package httpconst

import (
	"mime"
	"strings"
)

const (
	HeaderAuthorization  = "Authorization"
	HeaderContentType    = "Content-Type"
	HeaderForwardedFor   = "X-Forwarded-For"
	HeaderSessionID      = "X-Halaqaty-Session-ID"
	HeaderRequestID      = "X-Request-ID"
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
