package chat

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StagedUploadRetention is how long an unattached staged upload may exist
// before its object is revoked by cleanup.
const StagedUploadRetention = 24 * time.Hour

// StagedUploadSource atomically claims expired staged uploads for object
// cleanup.
type StagedUploadSource interface {
	// ClaimExpiredStaged transitions staged uploads created before cutoff to
	// revoked and returns the claimed rows. Claiming makes cleanup idempotent
	// and liveness-safe: a claimed row is never reprocessed even when its
	// object deletion fails.
	ClaimExpiredStaged(ctx context.Context, cutoff time.Time, limit int) ([]Upload, error)
	ReleaseExpiredStaged(ctx context.Context, uploadID uuid.UUID) error
	FinalizeExpiredStaged(ctx context.Context, uploadID uuid.UUID) error
}

// Cleaner revokes staged chat upload objects past retention and removes the
// staged object of an upload whose message finalization failed.
type Cleaner struct {
	source StagedUploadSource
	store  *MediaStore
	now    func() time.Time
}

// NewCleaner constructs a chat media cleaner.
func NewCleaner(source StagedUploadSource, store *MediaStore) *Cleaner {
	return &Cleaner{source: source, store: store, now: time.Now}
}

// CleanStaged revokes the objects of staged uploads older than
// StagedUploadRetention. The claim is the selection boundary: only rows that
// were staged past the cutoff are returned (see claimExpiredStagedUploadsQuery
// and its integration test), and a claimed row is never reprocessed even when
// its object deletion fails. Object deletion is best-effort per row: a
// failure is reported in the joined error without aborting the batch.
// Returned rows carry the post-transition 'revoked' state.
func (c *Cleaner) CleanStaged(ctx context.Context, limit int) error {
	if limit < 1 {
		return nil
	}
	cutoff := c.now().Add(-StagedUploadRetention)
	claimed, err := c.source.ClaimExpiredStaged(ctx, cutoff, limit)
	if err != nil {
		return fmt.Errorf("claim expired staged chat uploads: %w", err)
	}

	var failures []error
	for _, upload := range claimed {
		if err := c.store.ApplyDeleteMarker(ctx, upload.ObjectKey); err != nil {
			_ = c.source.ReleaseExpiredStaged(ctx, upload.ID)
			failures = append(failures, fmt.Errorf("delete staged chat upload object %s: %w", upload.ObjectKey, err))
			continue
		}
		if err := c.source.FinalizeExpiredStaged(ctx, upload.ID); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// CleanupFailedFinalization removes the staged object of an upload whose
// message finalization failed, so an orphaned upload does not linger. It is
// best-effort: a missing object deletes successfully (S3 semantics) and a
// non-staged upload is never touched. The caller must not let the returned
// error mask the original finalization failure.
func (c *Cleaner) CleanupFailedFinalization(ctx context.Context, upload Upload) error {
	if upload.State != UploadStateStaged {
		return nil
	}
	return c.store.ApplyDeleteMarker(ctx, upload.ObjectKey)
}

// PoolStagedUploadSource is the PostgreSQL-backed StagedUploadSource.
type PoolStagedUploadSource struct{ pool *pgxpool.Pool }

// NewPoolStagedUploadSource constructs the PostgreSQL-backed source.
func NewPoolStagedUploadSource(pool *pgxpool.Pool) *PoolStagedUploadSource {
	return &PoolStagedUploadSource{pool: pool}
}

// ClaimExpiredStaged atomically transitions up to limit staged uploads
// created before cutoff to revoked and returns the claimed rows.
func (s *PoolStagedUploadSource) ClaimExpiredStaged(ctx context.Context, cutoff time.Time, limit int) ([]Upload, error) {
	if limit < 1 {
		return []Upload{}, nil
	}
	rows, err := s.pool.Query(ctx, claimExpiredStagedUploadsQuery, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("claim expired staged chat uploads: %w", err)
	}
	defer rows.Close()

	uploads := []Upload{}
	for rows.Next() {
		upload, err := scanUpload(rows)
		if err != nil {
			return nil, fmt.Errorf("scan claimed staged chat upload: %w", err)
		}
		uploads = append(uploads, upload)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed staged chat uploads: %w", err)
	}
	return uploads, nil
}

// ReleaseExpiredStaged makes a failed object deletion retryable.
func (s *PoolStagedUploadSource) ReleaseExpiredStaged(ctx context.Context, uploadID uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, releaseExpiredStagedUploadQuery, uploadID); err != nil {
		return fmt.Errorf("release staged chat upload: %w", err)
	}
	return nil
}

// FinalizeExpiredStaged removes metadata after the object is revoked.
func (s *PoolStagedUploadSource) FinalizeExpiredStaged(ctx context.Context, uploadID uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, finalizeExpiredStagedUploadQuery, uploadID); err != nil {
		return fmt.Errorf("finalize staged chat upload cleanup: %w", err)
	}
	return nil
}
