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

// Sweep reconciles a bounded batch of attached message media after startup or
// during the periodic recovery loop.
func (r *MediaReconciler) Sweep(ctx context.Context, limit int) error {
	if r == nil || r.repo == nil || r.store == nil {
		return errors.New("reconcile chat media: dependencies are not configured")
	}
	ids, err := r.repo.FindMediaReconciliationMessageIDs(ctx, limit)
	if err != nil {
		return err
	}
	var firstErr error
	for _, id := range ids {
		if err := r.Reconcile(ctx, id); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("reconcile chat media message %s: %w", id, err)
		}
	}
	return firstErr
}
