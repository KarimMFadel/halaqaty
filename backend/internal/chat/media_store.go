package chat

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrMediaBucketMissing indicates the configured private chat bucket does
	// not exist. Startup fails fast; the bucket is never auto-created.
	ErrMediaBucketMissing = errors.New("chat: media bucket missing")
	// ErrMediaBucketNotVersioned indicates the private chat bucket exists but
	// versioning is not enabled. Startup fails fast; versioning is never
	// silently enabled by the application.
	ErrMediaBucketNotVersioned = errors.New("chat: media bucket not versioned")
)

// chatObjectKeyPrefix namespaces every private chat attachment object.
const chatObjectKeyPrefix = "chat"

// ObjectPutInput carries a server-derived key and validated media payload.
// It is an internal storage request, never a public upload DTO.
type ObjectPutInput struct {
	ObjectKey string
	MIMEType  string
	SizeBytes int64
	Body      io.Reader
}

// ObjectStore is the chat-owned private attachment storage boundary.
// Provider version identities remain inside the concrete adapter.
type ObjectStore interface {
	Put(context.Context, ObjectPutInput) error
	PresignGet(context.Context, string, time.Duration) (*url.URL, error)
	ApplyDeleteMarker(context.Context, string) error
	RemoveLatestDeleteMarker(context.Context, string) error
	EnsureChatBucketVersioned(context.Context) error
}

// MediaStore owns chat validation, server-generated keys and operation deadlines.
type MediaStore struct {
	store     ObjectStore
	opTimeout time.Duration
}

// NewMediaStore wraps the configured object store with chat policy.
// opTimeout bounds every storage operation and must be greater than zero.
func NewMediaStore(store ObjectStore, opTimeout time.Duration) *MediaStore {
	return &MediaStore{store: store, opTimeout: opTimeout}
}

// StageInput describes one private object put for a validated upload. The
// API deliberately accepts no filename: the object key and metadata are
// derived only from the server-generated upload ID and the server-detected
// MIME type.
type StageInput struct {
	UploadID  uuid.UUID
	Type      MessageType
	MIMEType  string
	SizeBytes int64
	Body      io.Reader
}

// chatObjectKey derives the private object key for a server-generated upload
// ID. It is the only key derivation; user filenames never participate.
func chatObjectKey(uploadID uuid.UUID) string {
	return chatObjectKeyPrefix + "/" + uploadID.String()
}

// allowedUploadMIME reports whether mime is in the media type's server-side
// allowlist (already lowercased and trimmed).
func allowedUploadMIME(mediaType MessageType, mime string) bool {
	_, ok := supportedMIMEs[mediaType][mime]
	return ok
}

// Stage stores one private chat attachment object and returns its object key.
// The content type is the sanitized allowlisted MIME type; no filename,
// content, or user metadata is ever attached to the object. The store applies
// its configured operation timeout to the put.
func (s *MediaStore) Stage(ctx context.Context, in StageInput) (string, error) {
	mime := strings.ToLower(strings.TrimSpace(in.MIMEType))
	if !allowedUploadMIME(in.Type, mime) {
		return "", ErrUnsupportedMIME
	}
	if in.SizeBytes <= 0 {
		return "", ErrUploadTooLarge
	}
	key := chatObjectKey(in.UploadID)

	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	if err := s.store.Put(ctx, ObjectPutInput{ObjectKey: key, MIMEType: mime, SizeBytes: in.SizeBytes, Body: in.Body}); err != nil {
		return "", fmt.Errorf("stage chat upload object: %w", err)
	}
	return key, nil
}

// PresignGet preserves the adapter's versionless access URL.
func (s *MediaStore) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	signed, err := s.store.PresignGet(ctx, objectKey, ttl)
	if err != nil {
		return nil, fmt.Errorf("presign chat upload object: %w", err)
	}
	return signed, nil
}

// ApplyDeleteMarker revokes versionless access while retaining stored bytes.
func (s *MediaStore) ApplyDeleteMarker(ctx context.Context, objectKey string) error {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	if err := s.store.ApplyDeleteMarker(ctx, objectKey); err != nil {
		return fmt.Errorf("apply chat upload delete marker: %w", err)
	}
	return nil
}

// RemoveLatestDeleteMarker recovers only server-generated chat upload keys.
// Exact marker version identity never crosses this policy boundary.
func (s *MediaStore) RemoveLatestDeleteMarker(ctx context.Context, objectKey string) error {
	if !isInternalChatObjectKey(objectKey) {
		return errors.New("find chat upload delete marker: object key is not an internal chat upload")
	}
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	if err := s.store.RemoveLatestDeleteMarker(ctx, objectKey); err != nil {
		return fmt.Errorf("recover chat upload delete marker: %w", err)
	}
	return nil
}
func isInternalChatObjectKey(objectKey string) bool {
	const prefix = chatObjectKeyPrefix + "/"
	if !strings.HasPrefix(objectKey, prefix) {
		return false
	}
	_, err := uuid.Parse(strings.TrimPrefix(objectKey, prefix))
	return err == nil
}

// EnsureChatBucketVersioned checks startup storage guarantees without creating
// a bucket or enabling versioning.
func (s *MediaStore) EnsureChatBucketVersioned(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	return s.store.EnsureChatBucketVersioned(ctx)
}
