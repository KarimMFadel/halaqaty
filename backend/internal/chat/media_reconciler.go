package chat

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultMediaReconciliationInterval = 30 * time.Second
	DefaultMediaReconciliationLimit    = 100

	// maxMediaReconciliationSweepPages bounds one sweep so a broken page query
	// can never spin forever. Keyset pagination already advances strictly, so
	// this is a defensive ceiling rather than the normal termination path.
	maxMediaReconciliationSweepPages = 1000
)

// MediaReconciler repairs the object-store/database boundary after a crash.
type MediaReconciler struct {
	repo  *Repository
	store *MediaStore
	mu    sync.Mutex
	after *uuid.UUID
}

// NewMediaReconciler constructs a media reconciliation worker.
func NewMediaReconciler(repo *Repository, store *MediaStore) *MediaReconciler {
	return &MediaReconciler{repo: repo, store: store}
}

// Reconcile applies the required state direction for one attached upload:
// deleted messages get a marker; active messages remove only the newest
// internal marker, leaving all retained object bytes untouched.
func (r *MediaReconciler) Reconcile(ctx context.Context, messageID uuid.UUID) error {
	if r == nil || r.repo == nil || r.store == nil {
		return errors.New("reconcile chat media: dependencies are not configured")
	}
	message, upload, err := r.repo.FindMessageUpload(ctx, messageID)
	if err != nil {
		return fmt.Errorf("load media reconciliation target: %w", err)
	}
	if message.State == MessageStateDeleted {
		return r.store.ApplyDeleteMarker(ctx, upload.ObjectKey)
	}
	return r.store.RemoveLatestDeleteMarker(ctx, upload.ObjectKey)
}

// Sweep reconciles attached message media in bounded keyset pages. Progress
// resumes across sweeps and wraps at the end so all rows are revisited.
func (r *MediaReconciler) Sweep(ctx context.Context, limit int) error {
	if r == nil || r.repo == nil || r.store == nil {
		return errors.New("reconcile chat media: dependencies are not configured")
	}
	if limit <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	after := r.after
	for page := 0; page < maxMediaReconciliationSweepPages; page++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(firstErr, err)
		}
		ids, err := r.repo.FindMediaReconciliationMessageIDs(ctx, after, limit)
		if err != nil {
			return errors.Join(firstErr, err)
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return errors.Join(firstErr, err)
			}
			if err := r.Reconcile(ctx, id); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("reconcile chat media message %s: %w", id, err)
			}
		}
		if len(ids) < limit {
			r.after = nil
			return firstErr
		}
		last := ids[len(ids)-1]
		after = &last
		r.after = after
	}
	return errors.Join(firstErr, fmt.Errorf("reconcile chat media: sweep exceeded %d pages", maxMediaReconciliationSweepPages))
}
