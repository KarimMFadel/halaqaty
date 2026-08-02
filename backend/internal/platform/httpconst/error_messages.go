package httpconst

const (
	ErrorMessageAuthMiddlewareNotConfigured = "auth middleware not configured"
	ErrorMessageInternalServerError         = "internal server error"
	ErrorMessageForbidden                   = "forbidden"
	ErrorMessageInvalidSession              = "invalid session"
	ErrorMessageMissingCircleID             = "missing circle id"
	ErrorMessageMissingSessionID            = "missing session id"
	ErrorMessageMissingOrInvalidBearerToken = "missing or invalid bearer token"
	ErrorMessageRequestTimeout              = "request timeout"
	ErrorMessageSessionExpired              = "session expired"
	ErrorMessageSessionRevoked              = "session revoked"
	ErrorMessageSessionUserMismatch         = "session user mismatch"
	ErrorMessageUnauthorized                = "unauthorized"
	ErrorMessageUnsupportedContentType      = "content-type must be application/json"
	ErrorMessageWebSocketConnLimitExceeded  = "websocket connection limit exceeded"
	ErrorMessageRateLimitExceeded           = "rate limit exceeded"

	ErrorMessageAuthMeNotImplemented           = "auth me endpoint is not implemented yet"
	ErrorMessageCircleRoleAssignNotImplemented = "circle role assignment endpoint is not implemented yet"
	ErrorMessageRoleMiddlewareNotConfigured    = "role middleware not configured"

	ErrorMessageValidationFailed         = "validation failed"
	ErrorMessageRequestBodyInvalid       = "request body must be valid json"
	ErrorMessageRequestBodyUnknownFields = "request body contains unknown fields"
	ErrorMessageDisplayNameRequired      = "display_name is required"
	ErrorMessageDisplayNameInvalid       = "display_name must be between 2 and 100 characters"
	ErrorMessagePreferredLanguageInvalid = "preferred_language must be one of: ar, en"
	ErrorMessageDeviceNameTooLong        = "device_name must be at most 100 characters"
	ErrorMessageEmailAlreadyRegistered   = "email is already registered to another account"
	ErrorMessageAuthHandlerNotConfigured = "auth handler not configured"
)
