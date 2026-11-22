package chat

import (
	"context"
	"errors"
	"fmt"
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

// Sweep reconciles every attached message media in bounded keyset pages after
// startup or during the periodic recovery loop. Paginating past each page keeps
// rows whose reconciliation keeps failing from starving later rows; the
// iteration bound guarantees termination.
func (r *MediaReconciler) Sweep(ctx context.Context, limit int) error {
	if r == nil || r.repo == nil || r.store == nil {
		return errors.New("reconcile chat media: dependencies are not configured")
	}
	if limit <= 0 {
		return nil
	}
	var firstErr error
	var after *uuid.UUID
	for page := 0; page < maxMediaReconciliationSweepPages; page++ {
		ids, err := r.repo.FindMediaReconciliationMessageIDs(ctx, after, limit)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := r.Reconcile(ctx, id); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("reconcile chat media message %s: %w", id, err)
			}
		}
		if len(ids) < limit {
			return firstErr
		}
		last := ids[len(ids)-1]
		after = &last
	}
	return fmt.Errorf("reconcile chat media: sweep exceeded %d pages", maxMediaReconciliationSweepPages)
}
