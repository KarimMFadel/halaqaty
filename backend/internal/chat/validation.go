package chat

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxTextRunes is the maximum trimmed text payload size.
	MaxTextRunes = 4000
	// MaxCursorLength is the maximum textual UUID cursor length.
	MaxCursorLength = 36
	// MaxIdempotencyKeyLength is the database and request key limit.
	MaxIdempotencyKeyLength = 128
	// MaxVoiceSizeBytes is the maximum voice-note size.
	MaxVoiceSizeBytes int64 = 20 * 1024 * 1024
	// MaxImageSizeBytes is the maximum JPEG/PNG image size.
	MaxImageSizeBytes int64 = 5 * 1024 * 1024
	// MaxFileSizeBytes is the maximum PDF size.
	MaxFileSizeBytes int64 = 10 * 1024 * 1024
	// MaxVoiceDurationSeconds is the maximum voice-note duration.
	MaxVoiceDurationSeconds = 300
)

var supportedMIMEs = map[MessageType]map[string]struct{}{
	MessageTypeVoice: {"audio/ogg": {}, "audio/mpeg": {}, "audio/mp4": {}, "audio/webm": {}},
	MessageTypeImage: {"image/jpeg": {}, "image/png": {}},
	MessageTypeFile:  {"application/pdf": {}},
}

// ValidateMessageInput validates context and payload compatibility.
func ValidateMessageInput(in MessageInput) error {
	if (in.CircleID == nil) == (in.DMRecipientID == nil) {
		return ErrInvalidContext
	}
	if in.DMRecipientID != nil && in.SenderID != uuid.Nil && *in.DMRecipientID == in.SenderID {
		return ErrInvalidContext
	}
	if strings.TrimSpace(in.MediaKey) != "" {
		return ErrUnsafeRequest
	}

	switch in.Type {
	case MessageTypeText:
		if _, err := ValidateText(in.Content); err != nil || in.UploadID != nil {
			return ErrInvalidPayload
		}
	case MessageTypeVoice, MessageTypeImage, MessageTypeFile:
		if in.UploadID == nil || strings.TrimSpace(in.Content) != "" {
			return ErrInvalidPayload
		}
	default:
		return ErrInvalidPayload
	}
	return nil
}

// ValidateText trims and validates a chat text payload.
func ValidateText(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > MaxTextRunes {
		return "", ErrInvalidText
	}
	return trimmed, nil
}

// ValidateCursor validates the optional UUID pagination cursor.
func ValidateCursor(cursor string) error {
	if cursor == "" || len(cursor) > MaxCursorLength {
		if cursor == "" {
			return nil
		}
		return ErrInvalidCursor
	}
	if _, err := uuid.Parse(cursor); err != nil {
		return ErrInvalidCursor
	}
	return nil
}

// ValidateIdempotencyKey validates a required bounded retry identity.
func ValidateIdempotencyKey(key string) error {
	if key == "" || len(key) > MaxIdempotencyKeyLength {
		return ErrInvalidIdempotencyKey
	}
	return nil
}

// ValidateUpload validates server-detected media type, size, and duration.
func ValidateUpload(in UploadInput) error {
	mimes, ok := supportedMIMEs[in.Type]
	if !ok {
		return ErrUnsupportedMIME
	}
	if _, ok := mimes[strings.ToLower(strings.TrimSpace(in.MIMEType))]; !ok {
		return ErrUnsupportedMIME
	}
	if in.SizeBytes <= 0 {
		return ErrUploadTooLarge
	}
	var maxSize int64
	switch in.Type {
	case MessageTypeVoice:
		maxSize = MaxVoiceSizeBytes
		if in.DurationSeconds < 1 || in.DurationSeconds > MaxVoiceDurationSeconds {
			return ErrInvalidDuration
		}
	case MessageTypeImage:
		maxSize = MaxImageSizeBytes
	case MessageTypeFile:
		maxSize = MaxFileSizeBytes
	}
	if in.DurationSeconds != 0 && in.Type != MessageTypeVoice {
		return ErrInvalidDuration
	}
	if in.SizeBytes > maxSize {
		return ErrUploadTooLarge
	}
	return nil
}
