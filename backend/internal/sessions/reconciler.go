package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// RecoveryCandidateLimit is the maximum number of candidates processed for
	// each lifecycle state in one sweep.
	RecoveryCandidateLimit = 25
	// RecoveryProviderTimeout bounds one provider operation in the worker.
	RecoveryProviderTimeout = 3 * time.Second
	// maxRecoveryPages bounds one keyset walk so a persistently failing page
	// cannot spin the worker forever.
	maxRecoveryPages = 100
)

// RecoveryStore is the small persistence port used by the bounded recovery
// worker. TrySessionLock must hold the transaction-scoped advisory lock until
// fn returns, so a provider operation cannot race a foreground lifecycle
// operation for the same session.
type RecoveryStore interface {
	ListRecoveryCandidates(context.Context, SessionStatus, int) ([]Session, error)
	TrySessionLock(context.Context, string, func(context.Context) error) (bool, error)
}

// recoveryCandidatePager is an optional RecoveryStore extension. Keyset
// pagination lets a sweep advance past candidates that never reach their
// completion condition instead of re-reading the same first page.
type recoveryCandidatePager interface {
	ListRecoveryCandidatesAfterID(ctx context.Context, status SessionStatus, afterID string, limit int) ([]Session, error)
}

type recoverySessionReader interface {
	GetSession(context.Context, string) (Session, error)
}

type attendanceRecovery interface {
	Finalize(context.Context, string) error
}

// Reconciler repairs the provider side of the session lifecycle without
// adding another source of durable state. Repeated sweeps are safe because
// provider operations are idempotent and PostgreSQL remains authoritative.
type Reconciler struct {
	store      RecoveryStore
	gateway    SessionMediaGateway
	roomKey    []byte
	clock      func() time.Time
	attendance attendanceRecovery
	mu         sync.Mutex
	cursors    map[SessionStatus]string
}

// SetAttendanceFinalizer enables recovery of ended sessions whose attendance was not finalized.
func (r *Reconciler) SetAttendanceFinalizer(finalizer attendanceRecovery) {
	r.attendance = finalizer
}

// NewReconciler constructs a bounded session reconciler. The key is backend
// configuration and must never be exposed to clients or logs.
func NewReconciler(store RecoveryStore, gateway SessionMediaGateway, roomKey []byte) (*Reconciler, error) {
	if store == nil || gateway == nil {
		return nil, errors.New("reconciler requires a store and media gateway")
	}
	if len(roomKey) == 0 {
		return nil, errors.New("reconciler room key is required")
	}
	return &Reconciler{store: store, gateway: gateway, roomKey: append([]byte(nil), roomKey...), clock: time.Now, cursors: make(map[SessionStatus]string)}, nil
}

// StableMediaRoomRef derives the stable opaque provider room reference for a
// session. It intentionally does not contain the session identifier.
func StableMediaRoomRef(sessionID string, roomKey []byte) (MediaRoomRef, error) {
	if sessionID == "" {
		return "", errors.New("session ID is required")
	}
	if len(roomKey) == 0 {
		return "", errors.New("room key is required")
	}
	mac := hmac.New(sha256.New, roomKey)
	_, _ = mac.Write([]byte("halaqaty/f005/media-room/" + sessionID))
	return MediaRoomRef("hlq-" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), nil
}

// Sweep performs one startup/periodic reconciliation pass. Phase and candidate
// failures are aggregated and returned while every remaining phase and
// candidate is still processed.
func (r *Reconciler) Sweep(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []error
	for _, status := range []SessionStatus{SessionStatusScheduled, SessionStatusActive, SessionStatusEnded} {
		if err := r.sweepStatus(ctx, status); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sweepStatus reconciles one lifecycle state. When the store supports keyset
// pagination it resumes after the last completed page across bounded sweeps,
// then wraps after exhaustion so all candidates are revisited.
func (r *Reconciler) sweepStatus(ctx context.Context, status SessionStatus) error {
	pager, paginated := r.store.(recoveryCandidatePager)
	var errs []error
	afterID := r.cursors[status]
	for page := 0; page < maxRecoveryPages; page++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		var candidates []Session
		var err error
		if paginated {
			candidates, err = pager.ListRecoveryCandidatesAfterID(ctx, status, afterID, RecoveryCandidateLimit)
		} else {
			candidates, err = r.store.ListRecoveryCandidates(ctx, status, RecoveryCandidateLimit)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("list %s recovery candidates: %w", status, err))
			break
		}
		for _, sess := range candidates {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(errs, err)...)
			}
			if err := r.reconcileCandidate(ctx, sess); err != nil {
				errs = append(errs, fmt.Errorf("reconcile session %s: %w", sess.ID, err))
			}
		}
		if !paginated || len(candidates) < RecoveryCandidateLimit {
			r.cursors[status] = ""
			break
		}
		afterID = candidates[len(candidates)-1].ID
		r.cursors[status] = afterID
	}
	return errors.Join(errs...)
}

// Run performs the startup sweep and then repeats it every 30 seconds until
// ctx is cancelled. The caller owns the goroutine and shutdown lifecycle. A
// failing sweep is logged and retried rather than disabling the worker, so
// persistent cleanup failures stay visible without stopping recovery.
func (r *Reconciler) Run(ctx context.Context) error {
	if err := r.Sweep(ctx); err != nil {
		slog.ErrorContext(ctx, "session reconciliation sweep failed", "error", err)
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.Sweep(ctx); err != nil {
				slog.ErrorContext(ctx, "session reconciliation sweep failed", "error", err)
			}
		}
	}
}

func (r *Reconciler) reconcileCandidate(ctx context.Context, sess Session) error {
	locked, err := r.store.TrySessionLock(ctx, sess.ID, func(lockCtx context.Context) error {
		// Candidate pages are snapshots. Re-read after acquiring the advisory
		// lock so a foreground lifecycle operation that committed after the
		// page was built cannot be acted on using stale state.
		current := sess
		if reader, ok := r.store.(recoverySessionReader); ok {
			var err error
			current, err = reader.GetSession(lockCtx, sess.ID)
			if err != nil {
				return fmt.Errorf("reread recovery candidate: %w", err)
			}
		}
		attemptCtx, cancel := context.WithTimeout(lockCtx, RecoveryProviderTimeout)
		defer cancel()
		roomRef, err := StableMediaRoomRef(current.ID, r.roomKey)
		if err != nil {
			return err
		}
		switch current.Status {
		case SessionStatusScheduled:
			// A scheduled row has no authoritative provider room. Closing the
			// deterministic candidate removes a room left by a create crash.
			return r.gateway.CloseRoom(attemptCtx, roomRef)
		case SessionStatusActive:
			if current.MediaRoomRef == "" {
				return errors.New("active session has no persisted media room reference")
			}
			return r.gateway.EnsureRoom(attemptCtx, current.MediaRoomRef, current.MediaMode)
		case SessionStatusEnded:
			if r.attendance != nil {
				if err := r.attendance.Finalize(attemptCtx, current.ID); err != nil {
					return fmt.Errorf("finalize recovered attendance: %w", err)
				}
			}
			if current.MediaRoomRef == "" {
				return nil
			}
			return r.gateway.CloseRoom(attemptCtx, current.MediaRoomRef)
		default:
			return nil
		}
	})
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	return nil
}
