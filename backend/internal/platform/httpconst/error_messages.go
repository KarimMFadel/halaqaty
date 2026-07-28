package httpconst

const (
	ErrorCodeNotImplemented = "ERR_NOT_IMPLEMENTED"
)

const (
	ErrorMessageAuthMiddlewareNotConfigured = "auth middleware not configured"
	ErrorMessageForbidden                   = "forbidden"
	ErrorMessageInvalidSession              = "invalid session"
	ErrorMessageMissingCircleID             = "missing circle id"
	ErrorMessageMissingSessionID            = "missing session id"
	ErrorMessageMissingOrInvalidBearerToken = "missing or invalid bearer token"
	ErrorMessageRequestTimeout              = "request timeout"
	ErrorMessageSessionExpired              = "session expired"
	ErrorMessageSessionUserMismatch         = "session user mismatch"
	ErrorMessageUnauthorized                = "unauthorized"
	ErrorMessageUnsupportedContentType      = "content-type must be application/json"
	ErrorMessageWebSocketConnLimitExceeded  = "websocket connection limit exceeded"
	ErrorMessageRateLimitExceeded           = "rate limit exceeded"

	ErrorMessageAuthMeNotImplemented                = "auth me endpoint is not implemented yet"
	ErrorMessageCircleRoleAssignNotImplemented      = "circle role assignment endpoint is not implemented yet"
	ErrorMessageRoleMiddlewareNotConfigured         = "role middleware not configured"
)
