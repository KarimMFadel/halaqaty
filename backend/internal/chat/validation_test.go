package chat

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateMessageInput(t *testing.T) {
	circleID := uuid.New()
	peerID := uuid.New()
	uploadID := uuid.New()

	tests := []struct {
		name string
		in   MessageInput
		want error
	}{
		{name: "valid group text", in: MessageInput{CircleID: &circleID, Type: MessageTypeText, Content: " salaam "}},
		{name: "valid direct media", in: MessageInput{DMRecipientID: &peerID, Type: MessageTypeImage, UploadID: &uploadID}},
		{name: "requires exactly one context", in: MessageInput{Type: MessageTypeText, Content: "hello"}, want: ErrInvalidContext},
		{name: "rejects both contexts", in: MessageInput{CircleID: &circleID, DMRecipientID: &peerID, Type: MessageTypeText, Content: "hello"}, want: ErrInvalidContext},
		{name: "rejects self direct message", in: MessageInput{DMRecipientID: &peerID, SenderID: peerID, Type: MessageTypeText, Content: "hello"}, want: ErrInvalidContext},
		{name: "text requires content and no upload", in: MessageInput{CircleID: &circleID, Type: MessageTypeText, UploadID: &uploadID}, want: ErrInvalidPayload},
		{name: "media requires upload and no content", in: MessageInput{CircleID: &circleID, Type: MessageTypeImage, Content: "hello"}, want: ErrInvalidPayload},
		{name: "rejects legacy media key", in: MessageInput{CircleID: &circleID, Type: MessageTypeImage, MediaKey: "objects/old", UploadID: &uploadID}, want: ErrUnsafeRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessageInput(tt.in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ValidateMessageInput() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want error
	}{
		{name: "trims surrounding whitespace", text: "  hello  "},
		{name: "accepts four thousand runes", text: strings.Repeat("م", MaxTextRunes)},
		{name: "rejects empty after trim", text: " \t\n", want: ErrInvalidText},
		{name: "rejects over four thousand runes", text: strings.Repeat("م", MaxTextRunes+1), want: ErrInvalidText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateText(tt.text)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ValidateText() error = %v, want %v", err, tt.want)
			}
			if tt.want == nil && got != strings.TrimSpace(tt.text) {
				t.Fatalf("ValidateText() = %q, want trimmed text", got)
			}
		})
	}
}

func TestValidateCursor(t *testing.T) {
	tests := []struct {
		name   string
		cursor string
		want   error
	}{
		{name: "optional cursor omitted"},
		{name: "valid UUID cursor", cursor: "550e8400-e29b-41d4-a716-446655440000"},
		{name: "rejects malformed cursor", cursor: "not-a-uuid", want: ErrInvalidCursor},
		{name: "rejects oversized cursor", cursor: strings.Repeat("a", MaxCursorLength+1), want: ErrInvalidCursor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateCursor(tt.cursor); !errors.Is(err, tt.want) {
				t.Fatalf("ValidateCursor() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateIdempotencyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want error
	}{
		{name: "accepts bounded key", key: "device-1-message-1"},
		{name: "rejects empty key", want: ErrInvalidIdempotencyKey},
		{name: "rejects key over contract limit", key: strings.Repeat("k", MaxIdempotencyKeyLength+1), want: ErrInvalidIdempotencyKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateIdempotencyKey(tt.key); !errors.Is(err, tt.want) {
				t.Fatalf("ValidateIdempotencyKey() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name string
		in   UploadInput
		want error
	}{
		{name: "valid voice", in: UploadInput{Type: MessageTypeVoice, MIMEType: "audio/webm", SizeBytes: MaxVoiceSizeBytes, DurationSeconds: MaxVoiceDurationSeconds}},
		{name: "valid image", in: UploadInput{Type: MessageTypeImage, MIMEType: "image/jpeg", SizeBytes: MaxImageSizeBytes}},
		{name: "valid PDF", in: UploadInput{Type: MessageTypeFile, MIMEType: "application/pdf", SizeBytes: MaxFileSizeBytes}},
		{name: "rejects unsupported MIME", in: UploadInput{Type: MessageTypeImage, MIMEType: "image/gif", SizeBytes: 1}, want: ErrUnsupportedMIME},
		{name: "rejects oversized voice", in: UploadInput{Type: MessageTypeVoice, MIMEType: "audio/ogg", SizeBytes: MaxVoiceSizeBytes + 1, DurationSeconds: 1}, want: ErrUploadTooLarge},
		{name: "rejects oversized image", in: UploadInput{Type: MessageTypeImage, MIMEType: "image/png", SizeBytes: MaxImageSizeBytes + 1}, want: ErrUploadTooLarge},
		{name: "rejects oversized PDF", in: UploadInput{Type: MessageTypeFile, MIMEType: "application/pdf", SizeBytes: MaxFileSizeBytes + 1}, want: ErrUploadTooLarge},
		{name: "rejects voice over duration", in: UploadInput{Type: MessageTypeVoice, MIMEType: "audio/mpeg", SizeBytes: 1, DurationSeconds: MaxVoiceDurationSeconds + 1}, want: ErrInvalidDuration},
		{name: "rejects duration on image", in: UploadInput{Type: MessageTypeImage, MIMEType: "image/png", SizeBytes: 1, DurationSeconds: 1}, want: ErrInvalidDuration},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateUpload(tt.in); !errors.Is(err, tt.want) {
				t.Fatalf("ValidateUpload() error = %v, want %v", err, tt.want)
			}
		})
	}
}
