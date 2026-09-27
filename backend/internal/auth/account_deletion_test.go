package auth

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	firebase "firebase.google.com/go/v4"
)

type accountDeletionStoreStub struct {
	mu       sync.Mutex
	uid      string
	closeErr error
	clearErr error
	listErr  error
	pending  []PendingFirebaseDeletion
	closed   []string
	cleared  []PendingFirebaseDeletion
}

func (s *accountDeletionStoreStub) CloseStudentAccount(_ context.Context, userID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = append(s.closed, userID)
	return s.uid, s.closeErr
}

func (s *accountDeletionStoreStub) ClearDeletedAccountFirebaseUID(_ context.Context, userID, firebaseUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleared = append(s.cleared, PendingFirebaseDeletion{UserID: userID, FirebaseUID: firebaseUID})
	return s.clearErr
}

func (s *accountDeletionStoreStub) ListPendingFirebaseDeletions(_ context.Context, afterID *string, limit int) ([]PendingFirebaseDeletion, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var eligible []PendingFirebaseDeletion
	for _, deletion := range s.pending {
		if afterID == nil || deletion.UserID > *afterID {
			eligible = append(eligible, deletion)
			if len(eligible) == limit {
				break
			}
		}
	}
	return eligible, nil
}

func TestAccountDeletionServiceRotatesPastFailingBatch(t *testing.T) {
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{
		{UserID: "first", FirebaseUID: "uid-1"},
		{UserID: "second", FirebaseUID: "uid-2"},
		{UserID: "third", FirebaseUID: "uid-3"},
	}}
	admin := &accountAdminStub{err: errors.New("Firebase unavailable")}
	service := NewAccountDeletionService(store, admin)
	if pending, err := service.ReconcilePending(context.Background(), 2); err != nil || pending != 2 {
		t.Fatalf("first page: pending=%d err=%v", pending, err)
	}
	if pending, err := service.ReconcilePending(context.Background(), 2); err != nil || pending != 1 {
		t.Fatalf("second page: pending=%d err=%v", pending, err)
	}
	if got := admin.uids; len(got) != 3 || got[2] != "uid-3" {
		t.Fatalf("later tombstone was starved: calls=%v", got)
	}
}

func TestAccountDeletionServiceRetryLogsSanitizedStage(t *testing.T) {
	var logs bytes.Buffer
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{{UserID: "local-user", FirebaseUID: "secret-firebase-uid"}}}
	service := NewAccountDeletionService(store, &accountAdminStub{err: errors.New("provider included secret-firebase-uid")})
	service.SetLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	if pending, err := service.ReconcilePending(context.Background(), 1); err != nil || pending != 1 {
		t.Fatalf("retry: pending=%d err=%v", pending, err)
	}
	if !strings.Contains(logs.String(), `"operation_id":"local-user"`) ||
		!strings.Contains(logs.String(), `"stage":"firebase_identity_removal"`) ||
		!strings.Contains(logs.String(), `"failure_class":"provider"`) ||
		strings.Contains(logs.String(), "secret-firebase-uid") {
		t.Fatalf("unsafe or incomplete retry log: %s", logs.String())
	}
}

func TestAccountDeletionServicePropagatesStoreFailureBeforeClosure(t *testing.T) {
	store := &accountDeletionStoreStub{closeErr: errors.New("db unavailable")}
	admin := &accountAdminStub{}
	service := NewAccountDeletionService(store, admin)
	_, err := service.Delete(context.Background(), "user-id", time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "close student account") || len(admin.uids) != 0 {
		t.Fatalf("pre-closure failure: err=%v Firebase calls=%v", err, admin.uids)
	}
}

func TestAccountDeletionServiceRejectsAlreadyTombstonedReplay(t *testing.T) {
	store := &accountDeletionStoreStub{closeErr: ErrAccountDeleted}
	admin := &accountAdminStub{}
	service := NewAccountDeletionService(store, admin)
	result, err := service.Delete(context.Background(), "user-id", time.Now().UTC())
	if !errors.Is(err, ErrAccountDeleted) || result != "" || len(admin.uids) != 0 {
		t.Fatalf("tombstone replay: result=%q err=%v Firebase calls=%v", result, err, admin.uids)
	}
}

func TestAccountDeletionServiceReconcileCountsFailures(t *testing.T) {
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{
		{UserID: "first", FirebaseUID: "first-uid"},
		{UserID: "second", FirebaseUID: "second-uid"},
	}}
	admin := &accountAdminStub{err: errors.New("Firebase unavailable")}
	service := NewAccountDeletionService(store, admin)
	pending, err := service.ReconcilePending(context.Background(), 2)
	if err != nil || pending != 2 {
		t.Fatalf("reconcile failures: pending=%d err=%v", pending, err)
	}
	store.listErr = errors.New("db unavailable")
	if _, err := service.ReconcilePending(context.Background(), 2); err == nil || !strings.Contains(err.Error(), "list pending Firebase deletions") {
		t.Fatalf("list failure: %v", err)
	}
}

type accountAdminStub struct {
	mu   sync.Mutex
	err  error
	uids []string
}

func (s *accountAdminStub) DeleteUser(_ context.Context, uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uids = append(s.uids, uid)
	return s.err
}

func TestAccountDeletionServiceRequiresVerifiedRecentAuthTime(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		authTime  time.Time
		wantError bool
	}{
		{name: "five minute boundary is valid", authTime: now.Add(-5 * time.Minute)},
		{name: "older auth time is rejected", authTime: now.Add(-5*time.Minute - time.Second), wantError: true},
		{name: "future auth time is rejected", authTime: now.Add(time.Second), wantError: true},
		{name: "missing auth time is rejected", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &accountDeletionStoreStub{uid: "firebase-user"}
			admin := &accountAdminStub{}
			service := NewAccountDeletionService(store, admin)
			service.nowFn = func() time.Time { return now }
			_, err := service.Delete(context.Background(), "user-id", tc.authTime)
			if tc.wantError != errors.Is(err, ErrRecentReauthentication) {
				t.Fatalf("Delete error = %v, want recent-auth error=%v", err, tc.wantError)
			}
			if tc.wantError && (len(store.closed) != 0 || len(admin.uids) != 0) {
				t.Fatalf("stale or missing auth_time reached mutation: closed=%v firebase=%v", store.closed, admin.uids)
			}
		})
	}
}

func TestAccountDeletionServiceReturnsPendingOnFirebaseFailure(t *testing.T) {
	store := &accountDeletionStoreStub{uid: "firebase-user"}
	admin := &accountAdminStub{err: errors.New("firebase unavailable")}
	service := NewAccountDeletionService(store, admin)
	service.nowFn = func() time.Time { return time.Now().UTC() }

	result, err := service.Delete(context.Background(), "user-id", service.nowFn())
	if err != nil || result != AccountDeletionPending {
		t.Fatalf("Delete: got (%q, %v), want pending", result, err)
	}
	if len(store.closed) != 1 || len(admin.uids) != 1 || len(store.cleared) != 0 {
		t.Fatalf("pending cleanup calls: closed=%v firebase=%v cleared=%v", store.closed, admin.uids, store.cleared)
	}
}

func TestAccountDeletionServiceClearsFirebaseUIDAfterRemoval(t *testing.T) {
	store := &accountDeletionStoreStub{uid: "firebase-user"}
	admin := &accountAdminStub{}
	service := NewAccountDeletionService(store, admin)
	service.nowFn = func() time.Time { return time.Now().UTC() }

	result, err := service.Delete(context.Background(), "user-id", service.nowFn())
	if err != nil || result != AccountDeletionComplete {
		t.Fatalf("Delete: got (%q, %v), want complete", result, err)
	}
	if len(store.cleared) != 1 || store.cleared[0] != (PendingFirebaseDeletion{UserID: "user-id", FirebaseUID: "firebase-user"}) {
		t.Fatalf("cleared Firebase identity: %v", store.cleared)
	}
}

func TestAccountDeletionServiceReconcilesPendingIdentityRemoval(t *testing.T) {
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{{UserID: "user-id", FirebaseUID: "firebase-user"}}}
	admin := &accountAdminStub{}
	service := NewAccountDeletionService(store, admin)

	pending, err := service.ReconcilePending(context.Background(), 10)
	if err != nil || pending != 0 {
		t.Fatalf("ReconcilePending: got (%d, %v), want no pending failures", pending, err)
	}
	if len(admin.uids) != 1 || len(store.cleared) != 1 {
		t.Fatalf("reconciliation calls: firebase=%v cleared=%v", admin.uids, store.cleared)
	}
}

func TestAccountDeletionServiceRetainsTombstoneDuringRetryOutage(t *testing.T) {
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{{UserID: "user-id", FirebaseUID: "firebase-user"}}}
	admin := &accountAdminStub{err: errors.New("firebase unavailable")}
	service := NewAccountDeletionService(store, admin)

	for range 2 {
		pending, err := service.ReconcilePending(context.Background(), 1)
		if err != nil || pending != 1 {
			t.Fatalf("ReconcilePending: got (%d, %v), want one pending", pending, err)
		}
	}
	admin.mu.Lock()
	deletionCalls := len(admin.uids)
	admin.mu.Unlock()
	store.mu.Lock()
	clearedCalls := len(store.cleared)
	store.mu.Unlock()
	if deletionCalls != 2 || clearedCalls != 0 {
		t.Fatalf("retry outage calls: firebase=%d cleared=%d", deletionCalls, clearedCalls)
	}
}

func TestAccountDeletionServiceConcurrentReconciliationIsIdempotent(t *testing.T) {
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{{UserID: "user-id", FirebaseUID: "firebase-user"}}}
	admin := &accountAdminStub{err: errors.New("firebase unavailable")}
	service := NewAccountDeletionService(store, admin)

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = service.ReconcilePending(context.Background(), 1)
		}()
	}
	wg.Wait()
	admin.mu.Lock()
	deletionCalls := len(admin.uids)
	admin.mu.Unlock()
	store.mu.Lock()
	clearedCalls := len(store.cleared)
	store.mu.Unlock()
	if deletionCalls != 2 || clearedCalls != 0 {
		t.Fatalf("concurrent retry calls: firebase=%d cleared=%d", deletionCalls, clearedCalls)
	}
}

func TestAccountDeletionServiceTreatsFirebaseUserNotFoundAsRemoved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"USER_NOT_FOUND"}}`))
	}))
	defer server.Close()
	t.Setenv("FIREBASE_AUTH_EMULATOR_HOST", strings.TrimPrefix(server.URL, "http://"))

	app, err := firebase.NewApp(context.Background(), &firebase.Config{ProjectID: "account-deletion-test"})
	if err != nil {
		t.Fatalf("create Firebase emulator app: %v", err)
	}
	admin, err := app.Auth(context.Background())
	if err != nil {
		t.Fatalf("create Firebase Auth client: %v", err)
	}
	store := &accountDeletionStoreStub{pending: []PendingFirebaseDeletion{{UserID: "user-id", FirebaseUID: "missing-user"}}}
	service := NewAccountDeletionService(store, admin)

	pending, err := service.ReconcilePending(context.Background(), 1)
	if err != nil || pending != 0 {
		t.Fatalf("ReconcilePending: got (%d, %v), want missing Firebase identity cleared", pending, err)
	}
	if len(store.cleared) != 1 || store.cleared[0].FirebaseUID != "missing-user" {
		t.Fatalf("cleared identities: %v", store.cleared)
	}
}
