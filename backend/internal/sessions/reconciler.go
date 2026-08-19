package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

const (
	// RecoveryCandidateLimit is the maximum number of candidates processed for
	// each lifecycle state in one sweep.
	RecoveryCandidateLimit = 25
	// RecoveryProviderTimeout bounds one provider operation in the worker.
	RecoveryProviderTimeout = 3 * time.Second
)

// RecoveryStore is the small persistence port used by the bounded recovery
// worker. TrySessionLock must hold the transaction-scoped advisory lock until
// fn returns, so a provider operation cannot race a foreground lifecycle
// operation for the same session.
type RecoveryStore interface {
	ListRecoveryCandidates(context.Context, SessionStatus, int) ([]Session, error)
	TrySessionLock(context.Context, string, func(context.Context) error) (bool, error)
}

type recoverySessionReader interface {
	GetSession(context.Context, string) (Session, error)
}

// Reconciler repairs the provider side of the session lifecycle without
// adding another source of durable state. Repeated sweeps are safe because
// provider operations are idempotent and PostgreSQL remains authoritative.
type Reconciler struct {
	store   RecoveryStore
	gateway SessionMediaGateway
	roomKey []byte
	clock   func() time.Time
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
	return &Reconciler{store: store, gateway: gateway, roomKey: append([]byte(nil), roomKey...), clock: time.Now}, nil
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

// Sweep performs one startup/periodic reconciliation pass. It returns the
// first operational error while continuing to process other candidates.
func (r *Reconciler) Sweep(ctx context.Context) error {
	var firstErr error
	for _, status := range []SessionStatus{SessionStatusScheduled, SessionStatusActive, SessionStatusEnded} {
		candidates, err := r.store.ListRecoveryCandidates(ctx, status, RecoveryCandidateLimit)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("list %s recovery candidates: %w", status, err)
			}
			continue
		}
		for _, sess := range candidates {
			if err := r.reconcileCandidate(ctx, sess); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("reconcile session: %w", err)
			}
		}
	}
	return firstErr
}

// Run performs the startup sweep and then repeats it every 30 seconds until
// ctx is cancelled. The caller owns the goroutine and shutdown lifecycle.
func (r *Reconciler) Run(ctx context.Context) error {
	// Startup failures are transient recovery work, not a reason to disable
	// the periodic worker; the next bounded sweep retries them.
	_ = r.Sweep(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// A transient provider or database failure is retried on the next
			// bounded sweep; it must not permanently disable recovery.
			_ = r.Sweep(ctx)
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
