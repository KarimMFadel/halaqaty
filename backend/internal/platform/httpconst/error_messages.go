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

	ErrorMessageAuthRegisterNotImplemented     = "auth register endpoint is not implemented yet"
	ErrorMessageAuthSessionsNotImplemented     = "auth sessions endpoint is not implemented yet"
	ErrorMessageAuthLogoutNotImplemented       = "auth logout endpoint is not implemented yet"
	ErrorMessageAuthMeNotImplemented           = "auth me endpoint is not implemented yet"
	ErrorMessageCircleRoleAssignNotImplemented = "circle role assignment endpoint is not implemented yet"
	ErrorMessageRoleMiddlewareNotConfigured    = "role middleware not configured"
)
