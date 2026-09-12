package chat

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MessageType identifies the durable payload carried by a chat message.
type MessageType string

const (
	// MessageTypeText is a plain-text chat message.
	MessageTypeText MessageType = "text"
	// MessageTypeVoice is a voice-note message backed by a chat upload.
	MessageTypeVoice MessageType = "voice"
	// MessageTypeImage is an image message backed by a chat upload.
	MessageTypeImage MessageType = "image"
	// MessageTypeFile is a PDF message backed by a chat upload.
	MessageTypeFile MessageType = "file"
)

// MessageState is the server-side lifecycle of a durable message.
type MessageState string

const (
	// MessageStateActive is a visible, non-deleted message.
	MessageStateActive MessageState = "active"
	// MessageStateDeleted is a soft-deleted message.
	MessageStateDeleted MessageState = "deleted"
)

// DeliveryStatus describes whether a server-accepted message is delivered or read.
// Pending and sent are local client states and are included for compatibility only.
type DeliveryStatus string

const (
	// DeliveryStatusPending is a local queued state.
	DeliveryStatusPending DeliveryStatus = "pending"
	// DeliveryStatusSent is a local transmitted-but-unconfirmed state.
	DeliveryStatusSent DeliveryStatus = "sent"
	// DeliveryStatusDelivered is the server-authoritative accepted state.
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	// DeliveryStatusRead is the server-authoritative state after an eligible read fact.
	DeliveryStatusRead DeliveryStatus = "read"
)

// UploadState is the lifecycle of a staged chat attachment.
type UploadState string

const (
	// UploadStateStaged is an inaccessible upload awaiting attachment.
	UploadStateStaged UploadState = "staged"
	// UploadStateAttached is an upload bound to one message.
	UploadStateAttached UploadState = "attached"
	// UploadStateRevoked is an upload whose application access was removed.
	UploadStateRevoked UploadState = "revoked"
)

// OutboxState is the delivery lifecycle of an identifier-only chat event.
type OutboxState string

const (
	// OutboxStatePending is ready for delivery or retry.
	OutboxStatePending OutboxState = "pending"
	// OutboxStateDelivered is projected successfully.
	OutboxStateDelivered OutboxState = "delivered"
	// OutboxStateParked is exhausted and awaiting explicit replay.
	OutboxStateParked OutboxState = "parked"
)

// ModerationAction identifies a durable moderation fact.
type ModerationAction string

const (
	// ModerationActionTeacherDelete records a teacher deletion.
	ModerationActionTeacherDelete ModerationAction = "teacher_delete"
)

// MessageInput is the untrusted payload shared by group and direct sends.
type MessageInput struct {
	SenderID       uuid.UUID
	CircleID       *uuid.UUID
	DMRecipientID  *uuid.UUID
	Type           MessageType
	Content        string
	UploadID       *uuid.UUID
	MediaKey       string
	ReplyToID      *uuid.UUID
	IdempotencyKey string
}

// UploadInput contains server-detected attachment metadata for validation.
type UploadInput struct {
	Type            MessageType
	MIMEType        string
	SizeBytes       int64
	DurationSeconds int
}

// Message is the persistence-oriented chat message value.
type Message struct {
	ID            uuid.UUID
	CircleID      *uuid.UUID
	DMRecipientID *uuid.UUID
	SenderID      uuid.UUID
	Type          MessageType
	Content       string
	UploadID      *uuid.UUID
	ReplyToID     *uuid.UUID
	State         MessageState
	SentAt        time.Time
	DeletedAt     *time.Time
}

// Upload is the persistence-oriented private chat attachment value.
type Upload struct {
	ID                    uuid.UUID
	UploaderID            uuid.UUID
	AuthorizationCircleID uuid.UUID
	DMPeerID              *uuid.UUID
	ObjectKey             string
	MIMEType              string
	OriginalFileName      string
	SizeBytes             int64
	DurationSeconds       int
	State                 UploadState
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// MessageRead records one idempotent recipient read fact.
type MessageRead struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	ReadAt    time.Time
}

// OutboxEvent stores only identifiers and delivery state for a chat event.
type OutboxEvent struct {
	EventID      uuid.UUID
	MessageID    uuid.UUID
	EventType    string
	RecipientID  *uuid.UUID
	State        OutboxState
	AttemptCount int
	AvailableAt  time.Time
	DeliveredAt  *time.Time
	ParkedAt     *time.Time
	// WasParked carries the pre-claim parked state of a replay-claimed event:
	// Postgres RETURNING yields post-update values (parked_at is already
	// NULL), so the replay claim selects this flag from the pre-update row.
	// Only ClaimReplayEvents populates it.
	WasParked bool
}

// ModerationAudit records a content-free teacher moderation action.
type ModerationAudit struct {
	ID         uuid.UUID
	MessageID  uuid.UUID
	CircleID   uuid.UUID
	ActorID    uuid.UUID
	Action     ModerationAction
	OccurredAt time.Time
}

var (
	// ErrInvalidContext indicates missing, duplicate, or self-DM context.
	ErrInvalidContext = errors.New("chat: invalid message context")
	// ErrInvalidPayload indicates incompatible message type and payload fields.
	ErrInvalidPayload = errors.New("chat: invalid message payload")
	// ErrInvalidText indicates text outside the trimmed 1-4000 rune range.
	ErrInvalidText = errors.New("chat: invalid text")
	// ErrInvalidCursor indicates a malformed or oversized pagination cursor.
	ErrInvalidCursor = errors.New("chat: invalid cursor")
	// ErrInvalidIdempotencyKey indicates an empty or oversized retry key.
	ErrInvalidIdempotencyKey = errors.New("chat: invalid idempotency key")
	// ErrIdempotencyConflict indicates a retry key reused for a different payload.
	ErrIdempotencyConflict = errors.New("chat: idempotency key payload conflict")
	// ErrUnsupportedMIME indicates a media type outside the approved allowlist.
	ErrUnsupportedMIME = errors.New("chat: unsupported MIME type")
	// ErrMalformedMedia indicates an allowlisted media family whose bytes fail
	// structural validation. HTTP maps this to contract status 422.
	ErrMalformedMedia = errors.New("chat: malformed media")
	// ErrUploadTooLarge indicates an attachment over its type-specific limit.
	ErrUploadTooLarge = errors.New("chat: upload too large")
	// ErrInvalidDuration indicates a missing, excessive, or incompatible duration.
	ErrInvalidDuration = errors.New("chat: invalid duration")
	// ErrUnsafeRequest indicates a legacy or unsafe compatibility field.
	ErrUnsafeRequest = errors.New("chat: unsafe request compatibility")
)
