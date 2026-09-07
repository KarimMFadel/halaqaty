package chat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
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

// objectClient is the narrow object-store surface the chat media store needs;
// it is satisfied by *minio.Client (asserted below) and by test fakes.
type objectClient interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	GetBucketVersioning(ctx context.Context, bucketName string) (minio.BucketVersioningConfiguration, error)
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

var _ objectClient = (*minio.Client)(nil)

// MediaStore is the narrow MinIO-backed private chat object store (ADR-021).
// Object keys are derived solely from server-generated upload IDs; user
// filenames never reach object paths or object metadata.
type MediaStore struct {
	client    objectClient
	bucket    string
	opTimeout time.Duration
}

// NewMediaStore constructs a chat media store. opTimeout bounds every
// object-store call as a context deadline; it must be > 0.
func NewMediaStore(client objectClient, bucket string, opTimeout time.Duration) *MediaStore {
	return &MediaStore{client: client, bucket: bucket, opTimeout: opTimeout}
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
	if _, err := s.client.PutObject(ctx, s.bucket, key, in.Body, in.SizeBytes, minio.PutObjectOptions{
		ContentType: mime,
	}); err != nil {
		return "", fmt.Errorf("stage chat upload object: %w", err)
	}
	return key, nil
}

// PresignGet returns a versionless presigned GET URL for objectKey. No
// versionId is ever included, so a later delete marker revokes the URL
// immediately while older versions remain retained (ADR-021).
func (s *MediaStore) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (*url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	signed, err := s.client.PresignedGetObject(ctx, s.bucket, objectKey, ttl, nil)
	if err != nil {
		return nil, fmt.Errorf("presign chat upload object: %w", err)
	}
	return signed, nil
}

// ApplyDeleteMarker writes a delete marker to objectKey by issuing a
// versionless delete on the versioned bucket. This immediately revokes every
// previously issued versionless presigned URL; physical bytes remain as an
// older version until a future approved retention feature (ADR-021).
func (s *MediaStore) ApplyDeleteMarker(ctx context.Context, objectKey string) error {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()
	if err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("apply chat upload delete marker: %w", err)
	}
	return nil
}

// EnsureChatBucketVersioned fails fast at startup unless the configured
// bucket exists AND has versioning enabled. It never auto-creates the bucket
// and never silently enables versioning: object-store misconfiguration must
// stop startup, not degrade revocation guarantees.
func (s *MediaStore) EnsureChatBucketVersioned(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.opTimeout)
	defer cancel()

	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("check chat media bucket: %w", err)
	}
	if !exists {
		return ErrMediaBucketMissing
	}
	versioning, err := s.client.GetBucketVersioning(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("read chat media bucket versioning: %w", err)
	}
	if !versioning.Enabled() {
		return ErrMediaBucketNotVersioned
	}
	return nil
}
