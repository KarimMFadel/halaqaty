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
	ErrorCodeMediaUnavailable    = "ERR_MEDIA_UNAVAILABLE"
)

// Queue error codes for the F-003 recitation queue. Conflict codes map to
// 409, validation codes to 422, and the audio-convergence code to 503 with
// the committed queue truth intact (spec §Security map).
const (
	// ErrorCodeQueueVersionConflict is returned when expected_version does not match the current queue version (409).
	ErrorCodeQueueVersionConflict = "ERR_QUEUE_VERSION_CONFLICT"
	// ErrorCodeQueueInvalidTransition is returned when the requested entry state transition is not permitted (409).
	ErrorCodeQueueInvalidTransition = "ERR_QUEUE_INVALID_TRANSITION"
	// ErrorCodeQueueNoWaitingEntry is returned when advance finds no waiting entry to select (409).
	ErrorCodeQueueNoWaitingEntry = "ERR_QUEUE_NO_WAITING_ENTRY"
	// ErrorCodeQueueEntryReciting is returned when the operation targets an entry that is currently reciting (409).
	ErrorCodeQueueEntryReciting = "ERR_QUEUE_ENTRY_RECITING"
	// ErrorCodeQueueEntryTerminal is returned when the operation targets an entry already in a terminal state (409).
	ErrorCodeQueueEntryTerminal = "ERR_QUEUE_ENTRY_TERMINAL"
	// ErrorCodeQueueRoundFinalized is returned when the round is finalized or permanently inert and accepts no changes (409).
	ErrorCodeQueueRoundFinalized = "ERR_QUEUE_ROUND_FINALIZED"
	// ErrorCodeQueueDuplicateCommand is returned when an idempotency key is reused with a different command (409).
	ErrorCodeQueueDuplicateCommand = "ERR_QUEUE_DUPLICATE_COMMAND"
	// ErrorCodeQueueInvalidEnum is returned when a queue enum value is not one of the supported options (422).
	ErrorCodeQueueInvalidEnum = "ERR_QUEUE_INVALID_ENUM"
	// ErrorCodeQueueInvalidRange is returned when a surah/ayah range is not a valid Quran range (422).
	ErrorCodeQueueInvalidRange = "ERR_QUEUE_INVALID_RANGE"
	// ErrorCodeQueueInvalidOrder is returned when an ordered-ids list is not a permutation of the candidates (422).
	ErrorCodeQueueInvalidOrder = "ERR_QUEUE_INVALID_ORDER"
	// ErrorCodeQueueInvalidGrade is returned when a grade is not one of the five accepted grades (422).
	ErrorCodeQueueInvalidGrade = "ERR_QUEUE_INVALID_GRADE"
	// ErrorCodeQueueInvalidNote is returned when a note exceeds the 500-character limit (422).
	ErrorCodeQueueInvalidNote = "ERR_QUEUE_INVALID_NOTE"
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
	FieldName              = "name"
	FieldTeacherUserIDs    = "teacher_user_ids"
	FieldBackupSupervisor  = "backup_supervisor_user_id"
	FieldInviteCode        = "invite_code"
	FieldDescription       = "description"
	FieldRules             = "rules"
	FieldMaxCapacity       = "max_capacity"
	FieldIsPrivate         = "is_private"
	FieldGenderRestriction = "gender_restriction"
	FieldLanguage          = "language"
	FieldGradingPolicy     = "grading_policy"
	FieldQuery             = "q"
	FieldDiscoverQuery     = "query"
	FieldCursor            = "cursor"
	// Queue request fields (F-003), including the five session policy fields
	// named as in the canonical queue contracts.
	FieldRoundType            = "round_type"
	FieldSurahID              = "surah_id"
	FieldFromAyah             = "from_ayah"
	FieldToAyah               = "to_ayah"
	FieldGradingRequired      = "grading_required"
	FieldOrderedIDs           = "ordered_ids"
	FieldNewPosition          = "new_position"
	FieldExpectedVersion      = "expected_version"
	FieldStatus               = "status"
	FieldGrade                = "grade"
	FieldNote                 = "note"
	FieldIdempotencyKey       = "idempotency_key"
	FieldDecision             = "decision"
	FieldQueuePopulation      = "population"
	FieldQueueFinalization    = "unfinished_finalization"
	FieldQueueOptOut          = "opt_out"
	FieldQueueGradeVisibility = "grade_visibility"
	FieldQueueGradeCorrection = "grade_correction"
)
