// Package notifications owns recipient notification history and delivery policy.
package notifications

import (
	"time"

	"github.com/google/uuid"
)

// EventType is an allowlisted product notification type.
type EventType string

const (
	EventSessionReminder      EventType = "session_reminder"
	EventGroupMessage         EventType = "group_message"
	EventPrivateMessage       EventType = "private_message"
	EventGradePosted          EventType = "grade_posted"
	EventCircleInvitation     EventType = "circle_invitation"
	EventQueueNextSoon        EventType = "queue_next_soon"
	EventQueueYourTurn        EventType = "queue_your_turn"
	EventStudentJoinedSession EventType = "student_joined_session"
	EventStudentJoinedCircle  EventType = "student_joined_circle"
	EventCircleArchived       EventType = "circle_archived"
	EventSessionStarted       EventType = "session_started"
	EventSessionEnded         EventType = "session_ended"
)

// SourceRefType identifies the source feature's authorized detail resource.
type SourceRefType string

const (
	SourceSession    SourceRefType = "session"
	SourceMessage    SourceRefType = "message"
	SourceRecitation SourceRefType = "recitation_log"
	SourceCircle     SourceRefType = "circle"
	SourceInvitation SourceRefType = "invitation"
	SourceQueue      SourceRefType = "queue"
)

// EventPreference describes a notification type's preference behavior.
type EventPreference struct {
	Type           EventType
	Configurable   bool
	DefaultEnabled bool
}

// SourceRef is an opaque pointer that clients must re-authorize through its source feature.
type SourceRef struct {
	Type SourceRefType `json:"type"`
	ID   uuid.UUID     `json:"id"`
}

// Item is the lock-screen-safe notification history projection.
type Item struct {
	ID        uuid.UUID  `json:"id"`
	Type      EventType  `json:"type"`
	SourceRef SourceRef  `json:"source_ref"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	IsRead    bool       `json:"is_read"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// Cursor is the stable descending history position.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Page contains a retained history page and its authoritative unread count.
type Page struct {
	Items       []Item  `json:"items"`
	UnreadCount int64   `json:"unread_count"`
	NextCursor  *Cursor `json:"next_cursor,omitempty"`
}

// TypeSetting is the effective preference and mutability metadata for one event type.
type TypeSetting struct {
	Type           EventType `json:"type"`
	Enabled        bool      `json:"enabled"`
	Configurable   bool      `json:"configurable"`
	DefaultEnabled bool      `json:"default_enabled"`
}

// QuietHours is the user's account-wide local quiet interval.
type QuietHours struct {
	Enabled    bool   `json:"enabled"`
	StartLocal string `json:"start_local"`
	EndLocal   string `json:"end_local"`
	Timezone   string `json:"timezone"`
}

// Preferences contains effective per-type settings and the profile timezone.
type Preferences struct {
	Types      []TypeSetting `json:"types"`
	QuietHours QuietHours    `json:"quiet_hours"`
}

// TypeSettingUpdate changes one configurable event-type preference.
type TypeSettingUpdate struct {
	Type    EventType
	Enabled bool
}

// QuietHoursUpdate changes the enabled state and local clock interval.
type QuietHoursUpdate struct {
	Enabled    bool
	StartLocal string
	EndLocal   string
}

// CreateInput is the approved source-owned event projection persisted as history and push intent.
type CreateInput struct {
	RecipientID        uuid.UUID
	Type               EventType
	SourceEventID      string
	SourceRef          SourceRef
	Title              string
	Body               string
	CreatedAt          *time.Time
	PushExpiresAt      time.Time
	PreferenceOverride bool
}

var eventPreferences = [...]EventPreference{
	{Type: EventSessionReminder, Configurable: true, DefaultEnabled: true},
	{Type: EventGroupMessage, Configurable: true, DefaultEnabled: true},
	{Type: EventPrivateMessage, DefaultEnabled: true},
	{Type: EventGradePosted, Configurable: true, DefaultEnabled: true},
	{Type: EventCircleInvitation, DefaultEnabled: true},
	{Type: EventQueueNextSoon, DefaultEnabled: true},
	{Type: EventQueueYourTurn, DefaultEnabled: true},
	{Type: EventStudentJoinedSession, Configurable: true, DefaultEnabled: true},
	{Type: EventStudentJoinedCircle, Configurable: true, DefaultEnabled: true},
	{Type: EventCircleArchived, Configurable: true, DefaultEnabled: true},
	{Type: EventSessionStarted, Configurable: true, DefaultEnabled: true},
	{Type: EventSessionEnded, Configurable: true, DefaultEnabled: true},
}

// EventPreferences returns the active event catalog in matrix order.
func EventPreferences() []EventPreference {
	return append([]EventPreference(nil), eventPreferences[:]...)
}
