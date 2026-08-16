// Package sessions owns the provider-neutral live-session domain: the
// circle-scoped session lifecycle, durable participant presence and hand
// state, and the session-media gateway boundary (F-005, ADR-015).
package sessions

import (
	"errors"
	"time"
)

// SessionStatus is the durable lifecycle state of a session. Transitions are
// scheduled → active → ended only.
type SessionStatus string

// Session lifecycle states (FR-002).
const (
	SessionStatusScheduled SessionStatus = "scheduled"
	SessionStatusActive    SessionStatus = "active"
	SessionStatusEnded     SessionStatus = "ended"
)

// EndReason is the durable attribution for a session ending. The zero value
// means the session has not ended.
type EndReason string

// Durable end attributions (FR-015).
const (
	EndReasonManual        EndReason = "manual"
	EndReasonDurationLimit EndReason = "duration_limit"
	EndReasonIdleTimeout   EndReason = "idle_timeout"
)

// MediaMode is the server-controlled media policy of a session (FR-024).
type MediaMode string

// Media modes; F-005 always creates audio_only sessions.
const (
	MediaModeAudioOnly  MediaMode = "audio_only"
	MediaModeAudioVideo MediaMode = "audio_video" // reserved for an approved future feature
)

// maxParticipants is the maximum number of concurrently present participants
// per session (FR-009); it mirrors the sessions.participant_count CHECK.
const maxParticipants = 50

// MediaRoomRef is the opaque provider room reference of an active session. It
// is never exposed in public session objects or realtime broadcasts (FR-003).
type MediaRoomRef string

// MediaCredential is an opaque participant-specific media credential. Its
// String method redacts the value so default formatting of a credential or
// its MediaConnection never leaks it; the raw value must still never be
// logged, persisted, cached, or broadcast (FR-005). As a string-kind type
// its JSON encoding is unaffected by the String method.
type MediaCredential string

// String returns a redacted placeholder so fmt %v/%+v and slog formatting
// cannot expose the credential value.
func (c MediaCredential) String() string { return "[REDACTED]" }

// GoString returns a redacted placeholder so %#v formatting cannot expose
// the credential value either.
func (c MediaCredential) GoString() string { return "[REDACTED]" }

// MediaConnection is the private per-participant media access returned by an
// authorized start or join. All three fields are required (FR-004).
type MediaConnection struct {
	// Endpoint is the trusted TLS media endpoint from server configuration.
	Endpoint string
	// Credential is the opaque identity-specific short-lived credential.
	Credential MediaCredential
	// ExpiresAt is the actual expiry of the signed credential.
	ExpiresAt time.Time
}

// Session is the durable circle-scoped live-session record.
type Session struct {
	ID               string
	CircleID         string
	CreatedBy        string
	Status           SessionStatus
	ScheduledAt      *time.Time // nil for F-005 ad-hoc rows; F-006 owns scheduling
	ActualStart      *time.Time
	ActualEnd        *time.Time
	EndReason        EndReason // "" while the session has not ended
	MediaMode        MediaMode
	MediaRoomRef     MediaRoomRef // "" until the session is activated
	IsLocked         bool
	ParticipantCount int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ParticipantPresence is the durable per-participant presence and hand-state
// record for one session (FR-010, FR-025).
type ParticipantPresence struct {
	SessionID          string
	UserID             string
	DisplayName        string
	FirstJoinedAt      *time.Time
	LastJoinedAt       *time.Time
	LastLeftAt         *time.Time
	ReconnectCount     int
	IsCurrentlyPresent bool
	RemovedAt          *time.Time
	HandRaisedAt       *time.Time // nil means the hand is lowered
}

// Lifecycle and participation errors. Handlers map these to HTTP statuses;
// they are compared with errors.Is.
var (
	// ErrSessionNotFound means no session exists for the identifier.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionNotStartable means the session state does not allow the
	// requested lifecycle transition or join (for example a still scheduled
	// session).
	ErrSessionNotStartable = errors.New("session is not in a startable state")
	// ErrSessionAlreadyActive means the session has already been started.
	ErrSessionAlreadyActive = errors.New("session is already active")
	// ErrSessionAlreadyEnded means the session has already ended; no further
	// lifecycle or participation transitions are possible.
	ErrSessionAlreadyEnded = errors.New("session has already ended")
	// ErrSessionFull means the session already holds the maximum number of
	// concurrently present participants.
	ErrSessionFull = errors.New("session is full")
	// ErrSessionLocked means the room lock blocks this new join.
	ErrSessionLocked = errors.New("session is locked")
	// ErrParticipantRemoved means the participant is not eligible to join,
	// reconnect, or act: they were removed, never joined, or are not
	// currently present.
	ErrParticipantRemoved = errors.New("participant is not eligible")
	// ErrNotCircleMember means the user lacks an active membership in the
	// session's circle; the service layer enforces it via circle_members.
	ErrNotCircleMember = errors.New("user is not an active member of the circle")
)
