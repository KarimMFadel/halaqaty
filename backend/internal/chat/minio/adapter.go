// Package minio is the sole backend MinIO object-store adapter (ADR-023):
// every MinIO SDK import in backend production code lives in this package.
// The chat domain depends only on the provider-neutral chat.ObjectStore
// boundary; provider version identities never cross it.
package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// objectClient is the narrow MinIO surface the adapter needs; it is
// satisfied by *minio.Client and by SDK fakes in tests.
type objectClient interface {
	BucketExists(ctx context.Context, bucketName string) (bool, error)
	GetBucketVersioning(ctx context.Context, bucketName string) (minio.BucketVersioningConfiguration, error)
	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
	ListObjects(ctx context.Context, bucketName string, opts minio.ListObjectsOptions) <-chan minio.ObjectInfo
}

var _ objectClient = (*minio.Client)(nil)

// Adapter implements chat.ObjectStore against the configured MinIO
// deployment. Object versioning identities, signing, and delete-marker
// mechanics stay inside this package (ADR-021/ADR-023).
type Adapter struct {
	client objectClient
	bucket string
}

var _ chat.ObjectStore = (*Adapter)(nil)

// NewAdapter constructs the MinIO adapter from validated chat-media
// configuration. It never dials storage and never creates a bucket; startup
// bucket validation is the separate explicit EnsureChatBucketVersioned call.
func NewAdapter(cfg config.ChatMediaConfig) (*Adapter, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init chat media object-store client: %w", err)
	}
	return &Adapter{client: client, bucket: cfg.Bucket}, nil
}

// Put stores one object carrying only server-derived metadata; filenames and
// user metadata never reach the object store. Operation context and error
// wrapping are owned by the chat policy wrapper.
func (a *Adapter) Put(ctx context.Context, in chat.ObjectPutInput) error {
	_, err := a.client.PutObject(ctx, a.bucket, in.ObjectKey, in.Body, in.SizeBytes, minio.PutObjectOptions{
		ContentType: in.MIMEType,
	})
	return err
}

// PresignGet returns a versionless presigned GET URL for objectKey. No
// versionId is ever included, so a later delete marker revokes the URL
// immediately while older versions remain retained (ADR-021).
func (a *Adapter) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (*url.URL, error) {
	return a.client.PresignedGetObject(ctx, a.bucket, objectKey, ttl, nil)
}

// EnsureChatBucketVersioned fails fast at startup unless the configured
// bucket exists AND has versioning enabled. It never auto-creates the bucket
// and never silently enables versioning: object-store misconfiguration must
// stop startup, not degrade revocation guarantees.
func (a *Adapter) EnsureChatBucketVersioned(ctx context.Context) error {
	exists, err := a.client.BucketExists(ctx, a.bucket)
	if err != nil {
		return fmt.Errorf("check chat media bucket: %w", err)
	}
	if !exists {
		return chat.ErrMediaBucketMissing
	}
	versioning, err := a.client.GetBucketVersioning(ctx, a.bucket)
	if err != nil {
		return fmt.Errorf("read chat media bucket versioning: %w", err)
	}
	if !versioning.Enabled() {
		return chat.ErrMediaBucketNotVersioned
	}
	return nil
}

// ApplyDeleteMarker ensures objectKey is shadowed by one versionless delete
// marker. It is idempotent when the latest version is already a marker.
func (a *Adapter) ApplyDeleteMarker(ctx context.Context, objectKey string) error {
	if chat.IsInternalObjectKey(objectKey) {
		if marker, err := a.latestDeleteMarker(ctx, objectKey); err != nil {
			return err
		} else if marker != "" {
			return nil
		}
	}
	return a.client.RemoveObject(ctx, a.bucket, objectKey, minio.RemoveObjectOptions{})
}

// RemoveDeleteMarker removes exactly the supplied internal version ID. It
// exists for infrastructure fixtures and adapter-internal recovery; version
// IDs never cross the neutral chat.ObjectStore contract.
func (a *Adapter) RemoveDeleteMarker(ctx context.Context, objectKey, versionID string) error {
	if !chat.IsInternalObjectKey(objectKey) {
		return errors.New("remove chat upload delete marker: object key is not an internal chat upload")
	}
	if strings.TrimSpace(versionID) == "" {
		return errors.New("remove chat upload delete marker: version id is required")
	}
	if err := a.client.RemoveObject(ctx, a.bucket, objectKey, minio.RemoveObjectOptions{VersionID: versionID}); err != nil {
		return fmt.Errorf("remove chat upload delete marker: %w", err)
	}
	return nil
}

// RemoveLatestDeleteMarker removes the latest internal marker by first
// resolving its exact version ID, allowing an active message to recover
// after a marker-before-commit crash without deleting an older object
// version. Absence of a marker is a no-op.
func (a *Adapter) RemoveLatestDeleteMarker(ctx context.Context, objectKey string) error {
	marker, err := a.latestDeleteMarker(ctx, objectKey)
	if err != nil || marker == "" {
		return err
	}
	return a.RemoveDeleteMarker(ctx, objectKey, marker)
}

// latestDeleteMarker resolves the exact version ID of the newest delete
// marker on objectKey, or "" when none exists. It deletes nothing and is the
// only place provider version identities are observed.
func (a *Adapter) latestDeleteMarker(ctx context.Context, objectKey string) (string, error) {
	if !chat.IsInternalObjectKey(objectKey) {
		return "", errors.New("find chat upload delete marker: object key is not an internal chat upload")
	}
	var latest *minio.ObjectInfo
	for object := range a.client.ListObjects(ctx, a.bucket, minio.ListObjectsOptions{Prefix: objectKey, Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			return "", fmt.Errorf("list chat upload versions: %w", object.Err)
		}
		if object.Key == objectKey && object.IsDeleteMarker && (latest == nil || object.LastModified.After(latest.LastModified)) {
			copy := object
			latest = &copy
		}
	}
	if latest == nil {
		return "", nil
	}
	return latest.VersionID, nil
}
