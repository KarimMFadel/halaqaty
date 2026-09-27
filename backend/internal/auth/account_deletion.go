package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	firebaseauth "firebase.google.com/go/v4/auth"
)

const (
	recentAuthenticationWindow = 5 * time.Minute
	firebaseDeletionTimeout    = 3 * time.Second
	defaultDeletionBatchSize   = 50
)

// ErrRecentReauthentication rejects missing, future, or stale Firebase auth_time.
var ErrRecentReauthentication = errors.New("recent Firebase reauthentication required")

// PendingFirebaseDeletion identifies a committed tombstone awaiting identity removal.
type PendingFirebaseDeletion struct {
	UserID      string
	FirebaseUID string
}

// AccountDeletionStore persists closure and pending identity cleanup.
type AccountDeletionStore interface {
	CloseStudentAccount(context.Context, string) (string, error)
	ClearDeletedAccountFirebaseUID(context.Context, string, string) error
	ListPendingFirebaseDeletions(context.Context, *string, int) ([]PendingFirebaseDeletion, error)
}

// FirebaseIdentityAdmin removes a Firebase Authentication identity.
type FirebaseIdentityAdmin interface {
	DeleteUser(context.Context, string) error
}

// AccountDeletionResult describes whether Firebase identity removal finished.
type AccountDeletionResult string

const (
	// AccountDeletionComplete means both local closure and Firebase removal finished.
	AccountDeletionComplete AccountDeletionResult = "complete"
	// AccountDeletionPending means local closure committed and Firebase cleanup will retry.
	AccountDeletionPending AccountDeletionResult = "pending_identity_removal"
)

// AccountDeletionService closes a local account before removing its Firebase identity.
type AccountDeletionService struct {
	store       AccountDeletionStore
	admin       FirebaseIdentityAdmin
	nowFn       func() time.Time
	logger      *slog.Logger
	reconcileMu sync.Mutex
	// ponytail: in-memory cursor resets on restart; persist scheduling if restarts repeatedly outpace hourly sweeps.
	cursor *string
}

// NewAccountDeletionService creates the student deletion flow.
func NewAccountDeletionService(store AccountDeletionStore, admin FirebaseIdentityAdmin) *AccountDeletionService {
	return &AccountDeletionService{store: store, admin: admin, nowFn: time.Now}
}

// SetLogger records sanitized cleanup stages for request and worker retries.
func (s *AccountDeletionService) SetLogger(logger *slog.Logger) { s.logger = logger }

// Delete commits local closure, then attempts Firebase identity removal.
func (s *AccountDeletionService) Delete(ctx context.Context, userID string, authTime time.Time) (AccountDeletionResult, error) {
	if s == nil || s.store == nil || s.admin == nil {
		return "", errors.New("account deletion service is not configured")
	}
	now := s.nowFn().UTC()
	if authTime.IsZero() || authTime.After(now) || now.Sub(authTime) > recentAuthenticationWindow {
		return "", ErrRecentReauthentication
	}
	firebaseUID, err := s.store.CloseStudentAccount(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("close student account: %w", err)
	}
	if firebaseUID == "" {
		return AccountDeletionPending, nil
	}
	if err := s.removeFirebaseIdentity(ctx, userID, firebaseUID); err != nil {
		return AccountDeletionPending, nil
	}
	return AccountDeletionComplete, nil
}

// ReconcilePending retries a bounded batch of committed Firebase removals.
func (s *AccountDeletionService) ReconcilePending(ctx context.Context, limit int) (int, error) {
	if s == nil || s.store == nil || s.admin == nil {
		return 0, errors.New("account deletion service is not configured")
	}
	if limit <= 0 {
		limit = defaultDeletionBatchSize
	}
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	pending, err := s.store.ListPendingFirebaseDeletions(ctx, s.cursor, limit)
	if err != nil {
		return 0, fmt.Errorf("list pending Firebase deletions: %w", err)
	}
	if len(pending) == 0 && s.cursor != nil {
		s.cursor = nil
		pending, err = s.store.ListPendingFirebaseDeletions(ctx, nil, limit)
		if err != nil {
			return 0, fmt.Errorf("list pending Firebase deletions: %w", err)
		}
	}
	var stillPending int
	for _, deletion := range pending {
		if deletion.FirebaseUID == "" || s.removeFirebaseIdentity(ctx, deletion.UserID, deletion.FirebaseUID) != nil {
			stillPending++
		}
	}
	if len(pending) > 0 {
		lastID := pending[len(pending)-1].UserID
		s.cursor = &lastID
	}
	return stillPending, nil
}

func (s *AccountDeletionService) removeFirebaseIdentity(ctx context.Context, userID, firebaseUID string) error {
	adminCtx, cancel := context.WithTimeout(ctx, firebaseDeletionTimeout)
	defer cancel()
	err := s.admin.DeleteUser(adminCtx, firebaseUID)
	if err != nil && !firebaseauth.IsUserNotFound(err) {
		s.logCleanupFailure(ctx, userID, "firebase_identity_removal", "provider")
		return err
	}
	if err := s.store.ClearDeletedAccountFirebaseUID(ctx, userID, firebaseUID); err != nil {
		s.logCleanupFailure(ctx, userID, "firebase_uid_clear", "store")
		return fmt.Errorf("clear deleted Firebase identity: %w", err)
	}
	return nil
}

func (s *AccountDeletionService) logCleanupFailure(ctx context.Context, userID, stage, failureClass string) {
	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, "account_deletion_cleanup_failed", "operation_id", userID, "stage", stage, "failure_class", failureClass)
}
