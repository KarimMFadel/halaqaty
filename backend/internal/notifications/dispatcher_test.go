package notifications_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

// The F-001 auth session repository satisfies the narrow invalid-token seam.
var _ notifications.DeviceTokenRemover = (*auth.SessionRepository)(nil)

type recordedRetry struct {
	notificationID uuid.UUID
	deviceTokenID  uuid.UUID
	nextAttemptAt  time.Time
	errorClass     string
	exhausted      bool
}

type recordedRemoval struct {
	userID string
	token  string
}

type fakeDispatchStore struct {
	intents       []notifications.DispatchIntent
	expired       int64
	hasAttempts   bool
	gates         notifications.DispatchGates
	gatesErr      error
	targets       []notifications.DeviceTarget
	suppressed    []notifications.DeliveryState
	accepted      []uuid.UUID
	retries       []recordedRetry
	settled       []uuid.UUID
	settleState   notifications.DeliveryState
	claimable     bool
	suppressCalls int
	failAttemptID uuid.UUID
	attemptErr    error
	// alreadyTerminal simulates a guarded suppression that matched zero rows.
	alreadyTerminal bool
}

func (s *fakeDispatchStore) ExpireDueIntents(_ context.Context, _ time.Time, _ int) (int64, error) {
	return s.expired, nil
}

func (s *fakeDispatchStore) ClaimDueIntents(_ context.Context, _ time.Time, _ int) ([]notifications.DispatchIntent, error) {
	if !s.claimable {
		return nil, nil
	}
	return s.intents, nil
}

func (s *fakeDispatchStore) HasDeviceAttempts(_ context.Context, id uuid.UUID) (bool, error) {
	if id == s.failAttemptID && s.attemptErr != nil {
		return false, s.attemptErr
	}
	return s.hasAttempts, nil
}

func (s *fakeDispatchStore) LoadDispatchGates(_ context.Context, _ uuid.UUID, _ notifications.EventType) (notifications.DispatchGates, error) {
	return s.gates, s.gatesErr
}

func (s *fakeDispatchStore) ListDueDeviceTargets(_ context.Context, _, _ uuid.UUID, _ time.Time) ([]notifications.DeviceTarget, error) {
	return s.targets, nil
}

func (s *fakeDispatchStore) SuppressIntent(_ context.Context, _, _ uuid.UUID, state notifications.DeliveryState) (bool, error) {
	s.suppressCalls++
	s.suppressed = append(s.suppressed, state)
	s.claimable = false // terminal suppression removes the intent from future claims
	return !s.alreadyTerminal, nil
}

func (s *fakeDispatchStore) RecordAccepted(_ context.Context, _, deviceTokenID uuid.UUID) error {
	s.accepted = append(s.accepted, deviceTokenID)
	return nil
}

func (s *fakeDispatchStore) RecordRetry(_ context.Context, notificationID, deviceTokenID uuid.UUID, nextAttemptAt time.Time, errorClass string, exhausted bool) error {
	s.retries = append(s.retries, recordedRetry{notificationID, deviceTokenID, nextAttemptAt, errorClass, exhausted})
	return nil
}

func (s *fakeDispatchStore) SettleNotification(_ context.Context, notificationID uuid.UUID) (notifications.DeliveryState, error) {
	s.settled = append(s.settled, notificationID)
	return s.settleState, nil
}

type fakePushSender struct {
	err  error
	sent []notifications.PushMessage
	hook func(message notifications.PushMessage) error
}

func (s *fakePushSender) Send(_ context.Context, message notifications.PushMessage) error {
	s.sent = append(s.sent, message)
	if s.hook != nil {
		return s.hook(message)
	}
	return s.err
}

type fakeTokenRemover struct {
	removed []recordedRemoval
}

func (r *fakeTokenRemover) DeleteDeviceToken(_ context.Context, userID, token string) error {
	r.removed = append(r.removed, recordedRemoval{userID, token})
	return nil
}

func testIntent() notifications.DispatchIntent {
	return notifications.DispatchIntent{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Type:      notifications.EventGroupMessage,
		SourceRef: notifications.SourceRef{Type: notifications.SourceMessage, ID: uuid.New()},
		Title:     "New message",
		Body:      "Open the app to view it",
	}
}

func newTestDispatcher(store *fakeDispatchStore, sender *fakePushSender, remover *fakeTokenRemover, metrics *notifications.NotificationMetrics, now time.Time) *notifications.Dispatcher {
	return notifications.NewDispatcher(store, sender, remover, metrics,
		func() time.Time { return now },
		func(d time.Duration) time.Duration { return d }, // identity jitter
	)
}

func TestPushRetryDelayIsExponentialJitteredAndCapped(t *testing.T) {
	identity := func(d time.Duration) time.Duration { return d }
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 4: 8 * time.Second, 5: 16 * time.Second} {
		if got := notifications.PushRetryDelay(attempt, identity); got != want {
			t.Errorf("PushRetryDelay(%d)=%s, want %s", attempt, got, want)
		}
	}
	if got := notifications.PushRetryDelay(12, identity); got != 30*time.Second {
		t.Errorf("PushRetryDelay(12)=%s, want 30s cap", got)
	}
	if got := notifications.PushRetryDelay(1, func(time.Duration) time.Duration { return -time.Second }); got != 0 {
		t.Errorf("negative jitter delay=%s, want clamped 0", got)
	}
	if got := notifications.PushRetryDelay(0, identity); got != time.Second {
		t.Errorf("PushRetryDelay(0)=%s, want first-attempt floor 1s", got)
	}
}

func TestClassifySendErrorIsSafeAndBounded(t *testing.T) {
	wrapped := fmt.Errorf("fcm send: %w", notifications.ErrInvalidDeviceToken)
	if got := notifications.ClassifySendError(wrapped); got != notifications.ErrorClassInvalidToken {
		t.Errorf("invalid token class=%q, want %q", got, notifications.ErrorClassInvalidToken)
	}
	if got := notifications.ClassifySendError(errors.New("provider exploded with sensitive detail")); got != notifications.ErrorClassTransientProvider {
		t.Errorf("transient class=%q, want %q", got, notifications.ErrorClassTransientProvider)
	}
}

func TestDispatchSuppressesPreferenceOffAtFirstAttempt(t *testing.T) {
	intent := testIntent()
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: false,
			QuietHours:        notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
	}
	sender := &fakePushSender{}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.suppressed) != 1 || store.suppressed[0] != notifications.DeliverySuppressedPreference {
		t.Fatalf("suppressed=%v, want one suppressed_preference", store.suppressed)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called %d times, want 0 for suppressed intent", len(sender.sent))
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchSuppressedPreference]; got != 1 {
		t.Fatalf("suppressed-preference metric=%d, want 1", got)
	}
}

func TestDispatchPreferenceReEnableDoesNotReplayOccurrence(t *testing.T) {
	intent := testIntent()
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: false,
			QuietHours:        notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
	}
	sender := &fakePushSender{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, &notifications.NotificationMetrics{}, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	// The user re-enables the type after the occurrence was suppressed.
	store.gates.PreferenceEnabled = true
	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called %d times after re-enable, want 0 (terminal suppression)", len(sender.sent))
	}
	if store.suppressCalls != 1 {
		t.Fatalf("suppress calls=%d, want exactly 1", store.suppressCalls)
	}
}

func TestDispatchQuietHoursSuppressesIncludingPreferenceOverride(t *testing.T) {
	intent := testIntent()
	intent.PreferenceOverride = true // manager-deletion notice bypasses preference, not quiet hours
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: true,
			QuietHours:        notifications.QuietHours{Enabled: true, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
	}
	sender := &fakePushSender{}
	metrics := &notifications.NotificationMetrics{}
	now := time.Date(2026, 10, 20, 23, 30, 0, 0, time.UTC)
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, metrics, now)

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.suppressed) != 1 || store.suppressed[0] != notifications.DeliverySuppressedQuietHours {
		t.Fatalf("suppressed=%v, want one suppressed_quiet_hours", store.suppressed)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called during quiet hours")
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchSuppressedQuietHours]; got != 1 {
		t.Fatalf("suppressed-quiet-hours metric=%d, want 1", got)
	}
}

func TestDispatchPreferenceOverrideBypassesPreferenceGateOnly(t *testing.T) {
	intent := testIntent()
	intent.PreferenceOverride = true
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: false, // ignored for override rows
			QuietHours:        notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "raw-token"}},
		settleState: notifications.DeliveryAccepted,
	}
	sender := &fakePushSender{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, &notifications.NotificationMetrics{}, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0].Token != "raw-token" {
		t.Fatalf("sent=%v, want one send to the device", sender.sent)
	}
	if len(store.accepted) != 1 || store.accepted[0] != tokenID {
		t.Fatalf("accepted=%v, want the device attempt accepted", store.accepted)
	}
}

func TestDispatchSkipsGatesOnceAttemptsExist(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true, // retry cycle: gates were evaluated at the first attempt
		gatesErr:    errors.New("gates must not be reloaded on retry"),
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "raw-token", AttemptCount: 1}},
		settleState: notifications.DeliveryAccepted,
	}
	sender := &fakePushSender{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, &notifications.NotificationMetrics{}, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent=%d, want retry send without gate re-evaluation", len(sender.sent))
	}
}

func TestDispatchTransientFailureSchedulesJitteredBackoff(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true,
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "raw-token", AttemptCount: 1}},
		settleState: notifications.DeliveryPendingAttempt,
	}
	sender := &fakePushSender{err: errors.New("fcm unavailable")}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, metrics, now)

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.retries) != 1 {
		t.Fatalf("retries=%v, want one scheduled retry", store.retries)
	}
	retry := store.retries[0]
	// Second failed attempt backs off 2s with identity jitter.
	if !retry.nextAttemptAt.Equal(now.Add(2 * time.Second)) {
		t.Fatalf("next_attempt_at=%s, want %s", retry.nextAttemptAt, now.Add(2*time.Second))
	}
	if retry.errorClass != notifications.ErrorClassTransientProvider || retry.exhausted {
		t.Fatalf("retry=(%q, exhausted=%t), want transient and not exhausted", retry.errorClass, retry.exhausted)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchRetried]; got != 1 {
		t.Fatalf("retried metric=%d, want 1", got)
	}
	if len(store.settled) != 1 {
		t.Fatalf("settle calls=%d, want 1", len(store.settled))
	}
}

func TestDispatchFifthFailureParksWithMetric(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true,
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "raw-token", AttemptCount: notifications.MaxPushAttempts - 1}},
		settleState: notifications.DeliveryExhausted,
	}
	sender := &fakePushSender{err: errors.New("fcm unavailable")}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.retries) != 1 || !store.retries[0].exhausted {
		t.Fatalf("retries=%v, want one exhausted (parked) record", store.retries)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchParked]; got != 1 {
		t.Fatalf("parked metric=%d, want 1", got)
	}
}

func TestDispatchInvalidTokenDeletesThroughRemoverSeam(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true,
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "stale-token"}},
		settleState: notifications.DeliveryPendingNoDevice,
	}
	sender := &fakePushSender{err: notifications.ErrInvalidDeviceToken}
	remover := &fakeTokenRemover{}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, sender, remover, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(remover.removed) != 1 || remover.removed[0].userID != intent.UserID.String() || remover.removed[0].token != "stale-token" {
		t.Fatalf("removed=%v, want the stale token deleted for the owner", remover.removed)
	}
	if len(store.retries) != 0 {
		t.Fatalf("retries=%v, want no retry for an invalid token", store.retries)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchInvalidToken]; got != 1 {
		t.Fatalf("invalid-token metric=%d, want 1", got)
	}
}

func TestDispatchWithoutDevicesSettlesBackToPendingNoDevice(t *testing.T) {
	intent := testIntent()
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: true,
			QuietHours:        notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
		targets:     nil, // token detached after wake-up
		settleState: notifications.DeliveryPendingNoDevice,
	}
	sender := &fakePushSender{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, &notifications.NotificationMetrics{}, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called without devices")
	}
	if len(store.settled) != 1 || store.settleState != notifications.DeliveryPendingNoDevice {
		t.Fatalf("settled=%v state=%q, want pending_no_device hold", store.settled, store.settleState)
	}
}

func TestDispatchDueSweepsExpiredIntents(t *testing.T) {
	store := &fakeDispatchStore{expired: 3}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, &fakePushSender{}, &fakeTokenRemover{}, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchExpired]; got != 3 {
		t.Fatalf("expired metric=%d, want 3", got)
	}
}

func TestDispatchAcceptedRecordsProviderAcceptanceOnly(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true,
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "raw-token"}},
		settleState: notifications.DeliveryAccepted,
	}
	sender := &fakePushSender{}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.accepted) != 1 {
		t.Fatalf("accepted=%v, want one accepted attempt", store.accepted)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchAccepted]; got != 1 {
		t.Fatalf("accepted metric=%d, want 1", got)
	}
	message := sender.sent[0]
	if message.Title != intent.Title || message.Body != intent.Body || message.NotificationID != intent.ID ||
		message.Type != intent.Type || message.SourceRef != intent.SourceRef {
		t.Fatalf("push message=%+v, want generic projection with typed source reference", message)
	}
}

func TestDispatchSuppressionMetricCountsOnlyRealTransitions(t *testing.T) {
	intent := testIntent()
	store := &fakeDispatchStore{
		intents:   []notifications.DispatchIntent{intent},
		claimable: true,
		gates: notifications.DispatchGates{
			PreferenceEnabled: false,
			QuietHours:        notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"},
		},
		alreadyTerminal: true, // guarded UPDATE matched zero rows (lost a race)
	}
	metrics := &notifications.NotificationMetrics{}
	d := newTestDispatcher(store, &fakePushSender{}, &fakeTokenRemover{}, metrics, time.Now())

	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(store.suppressed) != 1 {
		t.Fatalf("suppressed=%v, want the suppression attempt recorded", store.suppressed)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchSuppressedPreference]; got != 0 {
		t.Fatalf("suppressed-preference metric=%d, want 0 for a no-op guarded update", got)
	}
}

// wedgedPushSender blocks until its context is canceled, simulating a stuck
// provider call that only the per-send timeout can cut short.
type wedgedPushSender struct{}

func (wedgedPushSender) Send(ctx context.Context, _ notifications.PushMessage) error {
	<-ctx.Done()
	return ctx.Err()
}

// The bounded per-send context turns a wedged provider call into a transient
// failure: the attempt is rescheduled through the normal retry path and the
// batch completes instead of stalling.
func TestDispatchSendTimeoutSchedulesTransientRetry(t *testing.T) {
	intent := testIntent()
	tokenID := uuid.New()
	store := &fakeDispatchStore{
		intents:     []notifications.DispatchIntent{intent},
		claimable:   true,
		hasAttempts: true,
		targets:     []notifications.DeviceTarget{{DeviceTokenID: tokenID, Token: "wedged-token", AttemptCount: 1}},
		settleState: notifications.DeliveryPendingAttempt,
	}
	metrics := &notifications.NotificationMetrics{}
	d := notifications.NewDispatcher(store, wedgedPushSender{}, &fakeTokenRemover{}, metrics,
		func() time.Time { return time.Now() }, func(d time.Duration) time.Duration { return d })

	started := time.Now()
	if err := d.DispatchDue(context.Background(), 10); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	// Completion is only possible because the per-send timeout fired: the
	// wedged sender never returns on its own.
	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Fatalf("batch took %s, want the send cut short by the per-send timeout", elapsed)
	}
	if len(store.retries) != 1 {
		t.Fatalf("retries=%v, want one rescheduled attempt after the send timeout", store.retries)
	}
	if store.retries[0].errorClass != notifications.ErrorClassTransientProvider || store.retries[0].exhausted {
		t.Fatalf("retry=(%q, exhausted=%t), want transient and not exhausted",
			store.retries[0].errorClass, store.retries[0].exhausted)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchRetried]; got != 1 {
		t.Fatalf("retried metric=%d, want 1", got)
	}
}

func TestDispatchDueContinuesAfterOneIntentFails(t *testing.T) {
	first := testIntent()
	second := testIntent()
	failure := errors.New("source unavailable")
	store := &fakeDispatchStore{
		intents: []notifications.DispatchIntent{first, second}, claimable: true, failAttemptID: first.ID, attemptErr: failure,
		gates:   notifications.DispatchGates{PreferenceEnabled: true, QuietHours: notifications.QuietHours{Enabled: false, StartLocal: "22:00", EndLocal: "07:00", Timezone: "UTC"}},
		targets: []notifications.DeviceTarget{{DeviceTokenID: uuid.New(), Token: "fixture-token"}}, settleState: notifications.DeliveryAccepted,
	}
	sender := &fakePushSender{}
	d := newTestDispatcher(store, sender, &fakeTokenRemover{}, &notifications.NotificationMetrics{}, time.Now())
	if err := d.DispatchDue(context.Background(), 10); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want source failure", err)
	}
	if len(sender.sent) != 1 || sender.sent[0].NotificationID != second.ID {
		t.Fatalf("sent = %v, want later intent", sender.sent)
	}
}
