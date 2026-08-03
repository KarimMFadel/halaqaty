package httpconst

const (
	// ErrorCodeUnauthorized is returned when credentials are missing or invalid.
	ErrorCodeUnauthorized = "ERR_UNAUTHORIZED"
	// ErrorCodeSessionMissing is returned when the X-Halaqaty-Session-ID header is missing.
	ErrorCodeSessionMissing = "ERR_SESSION_MISSING"
	// ErrorCodeSessionNotFound is returned when the supplied session ID does not exist.
	ErrorCodeSessionNotFound = "ERR_SESSION_NOT_FOUND"
	// ErrorCodeSessionExpired is returned when the session absolute or inactivity expiry has passed.
	ErrorCodeSessionExpired = "ERR_SESSION_EXPIRED"
	// ErrorCodeSessionRevoked is returned when the session has been explicitly revoked.
	ErrorCodeSessionRevoked = "ERR_SESSION_REVOKED"
	// ErrorCodeSessionUserMismatch is returned when the session does not belong to the bearer-derived user.
	ErrorCodeSessionUserMismatch = "ERR_SESSION_USER_MISMATCH"
	// ErrorCodeForbidden is returned when the caller lacks permission for the action.
	ErrorCodeForbidden = "ERR_FORBIDDEN"
	// ErrorCodeRateLimitExceeded is returned when a request or websocket budget is exhausted.
	ErrorCodeRateLimitExceeded = "ERR_RATE_LIMIT_EXCEEDED"
	// ErrorCodeRequestTimeout is returned when a request exceeds the configured timeout.
	ErrorCodeRequestTimeout = "ERR_REQUEST_TIMEOUT"
	// ErrorCodeNotImplemented is returned by stub endpoints.
	ErrorCodeNotImplemented = "ERR_NOT_IMPLEMENTED"
	// ErrorCodeValidationFailed is returned when the request body or parameters fail validation.
	ErrorCodeValidationFailed = "ERR_VALIDATION_FAILED"
	// ErrorCodeConflict is returned when a request conflicts with an existing resource.
	ErrorCodeConflict = "ERR_CONFLICT"
	// ErrorCodeNotFound is returned when a referenced resource does not exist.
	ErrorCodeNotFound = "ERR_NOT_FOUND"
	// ErrorCodeInternalServerError is returned when an unexpected server error occurs.
	ErrorCodeInternalServerError = "ERR_INTERNAL_SERVER_ERROR"
)

// ValidationField names are used in the error envelope field map.
const (
	FieldAuthorization     = "authorization"
	FieldSessionID         = "session_id"
	FieldBody              = "body"
	FieldDisplayName       = "display_name"
	FieldPreferredLanguage = "preferred_language"
	FieldDeviceName        = "device_name"
	FieldFullName          = "full_name"
	FieldCountry           = "country"
	FieldBio               = "bio"
	FieldAvatarURL         = "avatar_url"
	FieldPhone             = "phone"
	FieldCircleID          = "circle_id"
	FieldUserID            = "user_id"
	FieldRole              = "role"
)
