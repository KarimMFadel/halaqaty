package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxPushAttempts is the per-device delivery-attempt ceiling before parking.
	MaxPushAttempts = 5
	// maxPushRetryDelay caps any single jittered retry delay.
	maxPushRetryDelay = 30 * time.Second
	// DefaultDispatchInterval is the periodic dispatch cadence used when Run
	// receives a non-positive interval.
	DefaultDispatchInterval = 5 * time.Second
	// DefaultDispatchLimit bounds one dispatch batch at MVP scale.
	DefaultDispatchLimit = 50
	// dispatchLeaseInterval is the fixed claim lease: a claimed intent is not
	// re-claimable until the lease expires, which prevents concurrent
	// double-dispatch across dispatcher instances and recovers rows orphaned
	// by a crash. It exceeds maxPushRetryDelay so normal retry scheduling
	// always shortens the lease.
	dispatchLeaseInterval = 60 * time.Second
	// pushSendTimeout bounds one provider send. A wedged FCM call must not
	// stall the batch, and the bound keeps the send window well inside
	// dispatchLeaseInterval so a slow send rarely overlaps a re-claim. A
	// timeout surfaces as a transient failure and takes the normal retry
	// path; delivery stays at-least-once (a crash between provider
	// acceptance and persistence can still re-send on retry).
	pushSendTimeout = 10 * time.Second
)

// Safe error classes persisted for operational diagnosis; raw provider
// payloads are never stored or logged.
const (
	// ErrorClassInvalidToken marks a token the provider rejected as unregistered.
	ErrorClassInvalidToken = "invalid_token"
	// ErrorClassTransientProvider marks a retryable provider/transport failure.
	ErrorClassTransientProvider = "transient_provider_error"
)

// ErrInvalidDeviceToken is returned (wrapped) by a PushSender when FCM rejects
// the registration token as unregistered; the token is then deleted through
// the F-001 token-lifecycle seam and never retried.
var ErrInvalidDeviceToken = errors.New("notifications: device token rejected as unregistered")

// DeliveryState is the push-delivery state of one notification occurrence.
// It describes push projection only and never claims handset receipt or read.
type DeliveryState string

const (
	// DeliveryPendingNoDevice holds the intent until token registration or expiry.
	DeliveryPendingNoDevice DeliveryState = "pending_no_device"
	// DeliveryPendingAttempt marks the intent eligible for dispatch attempts.
	DeliveryPendingAttempt DeliveryState = "pending_attempt"
	// DeliveryAccepted means at least one device send was provider-accepted.
	DeliveryAccepted DeliveryState = "accepted_by_provider"
	// DeliverySuppressedPreference is terminal: the type was OFF at first dispatch.
	DeliverySuppressedPreference DeliveryState = "suppressed_preference"
	// DeliverySuppressedQuietHours is terminal: quiet hours held at first dispatch.
	DeliverySuppressedQuietHours DeliveryState = "suppressed_quiet_hours"
	// DeliveryExpired is terminal: the push expiry passed without acceptance.
	DeliveryExpired DeliveryState = "expired"
	// DeliveryExhausted is terminal: every device attempt was parked.
	DeliveryExhausted DeliveryState = "exhausted"
)

// DispatchIntent is one claimed durable notification ready for dispatch work.
type DispatchIntent struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	Type               EventType
	SourceRef          SourceRef
	Title              string
	Body               string
	PushExpiresAt      time.Time
	PreferenceOverride bool
}

// DeviceTarget is one registered device due a send for a notification.
// Token is the raw FCM registration value: it is used only for the send and
// the owner-scoped deletion seam, and is never logged or persisted here.
type DeviceTarget struct {
	DeviceTokenID uuid.UUID
	Token         string
	AttemptCount  int
}

// DispatchGates carries the current preference and quiet-hours configuration
// evaluated immediately before the first push attempt of an occurrence.
type DispatchGates struct {
	PreferenceEnabled bool
	QuietHours        QuietHours
}

// PushMessage is the lock-screen-safe generic projection sent through FCM.
type PushMessage struct {
	Token          string
	Title          string
	Body           string
	NotificationID uuid.UUID
	Type           EventType
	SourceRef      SourceRef
}

// PushSender delivers one push. Implementations return a wrapped
// ErrInvalidDeviceToken for unregistered tokens; any other error is treated
// as transient and retried with bounded backoff. Real Firebase Admin wiring
// is feature task T014; tests inject fakes.
type PushSender interface {
	Send(ctx context.Context, message PushMessage) error
}

// DeviceTokenRemover deletes an invalid registration token through the F-001
// auth token-lifecycle boundary; auth.SessionRepository satisfies it.
type DeviceTokenRemover interface {
	DeleteDeviceToken(ctx context.Context, userID, token string) error
}

// DispatchStore is the persistence seam for durable dispatch state;
// *Repository implements it on PostgreSQL.
type DispatchStore interface {
	// ExpireDueIntents terminally expires pending intents past push_expires_at.
	ExpireDueIntents(ctx context.Context, now time.Time, limit int) (int64, error)
	// ClaimDueIntents leases due pending_attempt intents in bounded batches.
	ClaimDueIntents(ctx context.Context, now time.Time, limit int) ([]DispatchIntent, error)
	// HasDeviceAttempts reports whether any per-device attempt row exists yet.
	HasDeviceAttempts(ctx context.Context, notificationID uuid.UUID) (bool, error)
	// LoadDispatchGates resolves the current type preference and quiet hours.
	LoadDispatchGates(ctx context.Context, userID uuid.UUID, eventType EventType) (DispatchGates, error)
	// ListDueDeviceTargets returns registered devices whose attempt is due.
	ListDueDeviceTargets(ctx context.Context, notificationID, userID uuid.UUID, now time.Time) ([]DeviceTarget, error)
	// SuppressIntent terminally suppresses the occurrence and its device rows;
	// it reports whether the row actually transitioned (false when already
	// terminal), so metrics count only real suppressions.
	SuppressIntent(ctx context.Context, notificationID, userID uuid.UUID, state DeliveryState) (bool, error)
	// RecordAccepted marks one device attempt provider-accepted.
	RecordAccepted(ctx context.Context, notificationID, deviceTokenID uuid.UUID) error
	// RecordRetry reschedules one failed attempt, or parks it when exhausted.
	RecordRetry(ctx context.Context, notificationID, deviceTokenID uuid.UUID, nextAttemptAt time.Time, errorClass string, exhausted bool) error
	// SettleNotification recomputes the aggregate notification delivery state
	// from its per-device attempt rows and returns it.
	SettleNotification(ctx context.Context, notificationID uuid.UUID) (DeliveryState, error)
}

// Dispatcher projects durable notification intents to FCM with at most five
// attempts per device, jittered exponential backoff, terminal suppression and
// expiry states, invalid-token deletion, and a parking metric/log signal. It
// only processes rows already persisted by the creation path (persist-then-
// deliver) and never changes history, unread state, or source truth.
type Dispatcher struct {
	store   DispatchStore
	sender  PushSender
	remover DeviceTokenRemover
	metrics *NotificationMetrics
	now     func() time.Time
	jitter  func(time.Duration) time.Duration
}

// NewDispatcher constructs a dispatcher. now and jitter are injected to keep
// retry behavior deterministic in tests; nil selects the wall clock and the
// bounded ±10% default jitter.
func NewDispatcher(store DispatchStore, sender PushSender, remover DeviceTokenRemover, notificationMetrics *NotificationMetrics, now func() time.Time, jitter func(time.Duration) time.Duration) *Dispatcher {
	if now == nil {
		now = time.Now
	}
	if jitter == nil {
		jitter = defaultDispatchJitter
	}
	return &Dispatcher{store: store, sender: sender, remover: remover, metrics: notificationMetrics, now: now, jitter: jitter}
}

// DispatchDue expires stale intents, then claims and dispatches one bounded
// batch. The first store failure aborts the batch; the next cycle retries.
func (d *Dispatcher) DispatchDue(ctx context.Context, limit int) error {
	if d.store == nil || d.sender == nil || d.remover == nil {
		return errors.New("notifications dispatcher requires store, sender, and token remover")
	}
	if limit < 1 {
		limit = DefaultDispatchLimit
	}
	now := d.now().UTC()
	expired, err := d.store.ExpireDueIntents(ctx, now, limit)
	if err != nil {
		return fmt.Errorf("expire due notification intents: %w", err)
	}
	d.metrics.RecordDispatchBatch(DispatchExpired, expired)
	intents, err := d.store.ClaimDueIntents(ctx, now, limit)
	if err != nil {
		return fmt.Errorf("claim due notification intents: %w", err)
	}
	for _, intent := range intents {
		if err := d.dispatchIntent(ctx, intent); err != nil {
			return err
		}
	}
	return nil
}

// Run drives periodic dispatch until ctx is canceled. Per-cycle failures are
// tolerated: durable intent rows are reclaimed by the next cycle. Cycle
// failures are logged (no tokens or provider payloads); expected cancellation
// during shutdown is not.
func (d *Dispatcher) Run(ctx context.Context, limit int, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultDispatchInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := d.DispatchDue(ctx, limit); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "notification dispatch cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// dispatchIntent applies the first-attempt gates, sends to due devices, and
// settles the aggregate state from per-device outcomes.
func (d *Dispatcher) dispatchIntent(ctx context.Context, intent DispatchIntent) error {
	hasAttempts, err := d.store.HasDeviceAttempts(ctx, intent.ID)
	if err != nil {
		return fmt.Errorf("check notification device attempts: %w", err)
	}
	if !hasAttempts {
		// First push attempt for this occurrence: evaluate the current gates.
		// Suppression is terminal and is never replayed by later changes.
		gates, err := d.store.LoadDispatchGates(ctx, intent.UserID, intent.Type)
		if err != nil {
			return fmt.Errorf("load dispatch gates: %w", err)
		}
		if !intent.PreferenceOverride && !gates.PreferenceEnabled {
			return d.suppress(ctx, intent, DeliverySuppressedPreference)
		}
		quiet, err := IsQuietHours(gates.QuietHours, d.now())
		if err != nil {
			return fmt.Errorf("evaluate quiet hours: %w", err)
		}
		if quiet {
			return d.suppress(ctx, intent, DeliverySuppressedQuietHours)
		}
	}
	targets, err := d.store.ListDueDeviceTargets(ctx, intent.ID, intent.UserID, d.now().UTC())
	if err != nil {
		return fmt.Errorf("list due device targets: %w", err)
	}
	for _, target := range targets {
		if err := d.sendToDevice(ctx, intent, target); err != nil {
			return err
		}
	}
	if _, err := d.store.SettleNotification(ctx, intent.ID); err != nil {
		return fmt.Errorf("settle notification delivery state: %w", err)
	}
	return nil
}

// suppress terminally marks the occurrence; history and unread state stay.
// The metric counts only rows that actually transitioned to the terminal
// suppression state.
func (d *Dispatcher) suppress(ctx context.Context, intent DispatchIntent, state DeliveryState) error {
	suppressed, err := d.store.SuppressIntent(ctx, intent.ID, intent.UserID, state)
	if err != nil {
		return fmt.Errorf("suppress notification intent: %w", err)
	}
	if !suppressed {
		return nil
	}
	if state == DeliverySuppressedPreference {
		d.metrics.RecordDispatch(DispatchSuppressedPreference)
	} else {
		d.metrics.RecordDispatch(DispatchSuppressedQuietHours)
	}
	return nil
}

// sendToDevice performs one bounded per-device attempt. Provider acceptance is
// recorded as acceptance only; invalid tokens are deleted through the F-001
// seam; transient failures reschedule or park at the five-attempt ceiling.
func (d *Dispatcher) sendToDevice(ctx context.Context, intent DispatchIntent, target DeviceTarget) error {
	// The per-send context bounds the provider call; the parent ctx is what
	// detects shutdown cancellation below, so a send timeout takes the
	// transient retry path instead of aborting the batch.
	sendCtx, cancelSend := context.WithTimeout(ctx, pushSendTimeout)
	sendErr := d.sender.Send(sendCtx, PushMessage{
		Token:          target.Token,
		Title:          intent.Title,
		Body:           intent.Body,
		NotificationID: intent.ID,
		Type:           intent.Type,
		SourceRef:      intent.SourceRef,
	})
	cancelSend()
	if sendErr == nil {
		if err := d.store.RecordAccepted(ctx, intent.ID, target.DeviceTokenID); err != nil {
			return fmt.Errorf("record accepted device attempt: %w", err)
		}
		d.metrics.RecordDispatch(DispatchAccepted)
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("dispatch canceled: %w", sendErr)
	}
	if errors.Is(sendErr, ErrInvalidDeviceToken) {
		if err := d.remover.DeleteDeviceToken(ctx, intent.UserID.String(), target.Token); err != nil {
			return fmt.Errorf("delete invalid device token: %w", err)
		}
		d.metrics.RecordDispatch(DispatchInvalidToken)
		slog.WarnContext(ctx, "deleted unregistered device token",
			"notification_id", intent.ID, "device_token_id", target.DeviceTokenID)
		return nil
	}
	attempts := target.AttemptCount + 1
	exhausted := attempts >= MaxPushAttempts
	nextAttemptAt := d.now().UTC().Add(PushRetryDelay(attempts, d.jitter))
	if err := d.store.RecordRetry(ctx, intent.ID, target.DeviceTokenID, nextAttemptAt, ClassifySendError(sendErr), exhausted); err != nil {
		return fmt.Errorf("record failed device attempt: %w", err)
	}
	if exhausted {
		d.metrics.RecordDispatch(DispatchParked)
		slog.WarnContext(ctx, "notification device attempt exhausted; parked",
			"notification_id", intent.ID, "device_token_id", target.DeviceTokenID)
		return nil
	}
	d.metrics.RecordDispatch(DispatchRetried)
	return nil
}

// PushRetryDelay computes the jittered exponential backoff after the failed
// attempt n (1-based): 1/2/4/8/16 seconds, capped at 30 seconds.
func PushRetryDelay(attempt int, jitter func(time.Duration) time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1) << (attempt - 1) * time.Second
	if jitter != nil {
		delay = jitter(delay)
	}
	if delay < 0 {
		delay = 0
	}
	if delay > maxPushRetryDelay {
		delay = maxPushRetryDelay
	}
	return delay
}

// ClassifySendError maps a provider failure to a safe bounded error class;
// raw provider messages are never persisted or logged.
func ClassifySendError(err error) string {
	if errors.Is(err, ErrInvalidDeviceToken) {
		return ErrorClassInvalidToken
	}
	return ErrorClassTransientProvider
}

// defaultDispatchJitter applies bounded ±10% jitter to avoid synchronized
// retry bursts while preserving the configured exponential schedule.
func defaultDispatchJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	delta := delay / 10
	if delta == 0 {
		return delay
	}
	return delay - delta + time.Duration(rand.Int63n(int64(2*delta)+1))
}
