package chat

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"time"
)

// InsertUploadWithinBudget atomically accepts one staged upload within the
// per-user rolling budget. Object I/O occurs before this short transaction;
// losing concurrent uploads are cleaned up by the service.
func (r *Repository) InsertUploadWithinBudget(ctx context.Context, upload Upload, since time.Time) (Upload, error) {
	var staged Upload
	err := r.WithTx(ctx, func(tx *Tx) error {
		if _, err := tx.tx.Exec(ctx, lockUploadBudgetQuery, upload.UploaderID.String()); err != nil {
			return fmt.Errorf("lock upload budget: %w", err)
		}
		var count int
		if err := tx.tx.QueryRow(ctx, countRecentUploadsQuery, upload.UploaderID, since).Scan(&count); err != nil {
			return fmt.Errorf("count upload budget: %w", err)
		}
		if count >= MaxUploadsPerRollingHour {
			return ErrUploadRateExceeded
		}
		var id, duration any
		if upload.ID != uuid.Nil {
			id = upload.ID
		}
		if upload.DurationSeconds != 0 {
			duration = upload.DurationSeconds
		}
		var err error
		staged, err = scanUpload(tx.tx.QueryRow(ctx, insertUploadQuery, id, upload.UploaderID, upload.AuthorizationCircleID, upload.DMPeerID, upload.ObjectKey, upload.MIMEType, upload.OriginalFileName, upload.SizeBytes, duration))
		if err != nil {
			return fmt.Errorf("insert budgeted chat upload: %w", err)
		}
		return nil
	})
	return staged, err
}
