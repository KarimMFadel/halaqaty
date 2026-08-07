//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
)

const (
	profileFlowFirebaseUID = "firebase-profile-flow-001"
	profileFlowEmail       = "profile-flow@halaqaty.app"
	profileFlowUserID      = "66666666-6666-6666-6666-666666666666"
	profileFlowBearer      = flowBearer
)

func TestProfileFlow_ReadUpdateRead(t *testing.T) {
	store := newProfileFlowStore()
	verifier := &flowVerifier{
		decoded: &auth.DecodedToken{
			UID:   profileFlowFirebaseUID,
			Email: profileFlowEmail,
		},
	}

	authService := auth.NewService(store, nil, 24*time.Hour)
	authHandler := auth.NewHandler(authService)
	profileService := profile.NewService(store)
	profileHandler := profile.NewHandler(profileService)

	sessionService := auth.NewSessionService(24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, store)

	mux := http.NewServeMux()
	mux.Handle("POST /auth/register", authMW.RequireVerifiedFirebase(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /auth/sessions", authMW.RequireBearer(http.HandlerFunc(authHandler.CreateSession)))
	mux.Handle("GET /auth/me", authMW.Require(http.HandlerFunc(profileHandler.GetMe)))
	mux.Handle("PUT /auth/me", authMW.Require(http.HandlerFunc(profileHandler.UpdateMe)))

	registerResp := doJSONRequest(t, mux, http.MethodPost, "/auth/register", `{"display_name":"Ali"}`, map[string]string{
		httpconst.HeaderAuthorization: profileFlowBearer,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status: got %d want %d body=%s", registerResp.Code, http.StatusCreated, registerResp.Body.String())
	}

	createSessionResp := doJSONRequest(t, mux, http.MethodPost, "/auth/sessions", `{"device_name":"ios"}`, map[string]string{
		httpconst.HeaderAuthorization: profileFlowBearer,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if createSessionResp.Code != http.StatusOK {
		t.Fatalf("create session status: got %d want %d body=%s", createSessionResp.Code, http.StatusOK, createSessionResp.Body.String())
	}

	var session auth.BackendSessionResponse
	if err := json.Unmarshal(createSessionResp.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session response: %v", err)
	}

	firstRead := doJSONRequest(t, mux, http.MethodGet, "/auth/me", "", map[string]string{
		httpconst.HeaderAuthorization: profileFlowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
	})
	if firstRead.Code != http.StatusOK {
		t.Fatalf("first read status: got %d want %d body=%s", firstRead.Code, http.StatusOK, firstRead.Body.String())
	}

	update := doJSONRequest(t, mux, http.MethodPut, "/auth/me", `{"full_name":"Ali Mahmoud","country":"eg","bio":"Student"}`, map[string]string{
		httpconst.HeaderAuthorization: profileFlowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if update.Code != http.StatusOK {
		t.Fatalf("update status: got %d want %d body=%s", update.Code, http.StatusOK, update.Body.String())
	}

	secondRead := doJSONRequest(t, mux, http.MethodGet, "/auth/me", "", map[string]string{
		httpconst.HeaderAuthorization: profileFlowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
	})
	if secondRead.Code != http.StatusOK {
		t.Fatalf("second read status: got %d want %d body=%s", secondRead.Code, http.StatusOK, secondRead.Body.String())
	}

	var got auth.UserProfile
	if err := json.Unmarshal(secondRead.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if got.FullName == nil || *got.FullName != "Ali Mahmoud" {
		t.Fatalf("expected full_name Ali Mahmoud, got %+v", got.FullName)
	}
	if got.Country == nil || *got.Country != "EG" {
		t.Fatalf("expected country EG, got %+v", got.Country)
	}
	if got.Bio == nil || *got.Bio != "Student" {
		t.Fatalf("expected bio Student, got %+v", got.Bio)
	}
}

type profileFlowStore struct {
	mu          sync.Mutex
	userByUID   map[string]auth.User
	userIDByUID map[string]string
	profiles    map[string]profile.Record
	sessions    map[string]auth.Session
}

func newProfileFlowStore() *profileFlowStore {
	return &profileFlowStore{
		userByUID:   make(map[string]auth.User),
		userIDByUID: make(map[string]string),
		profiles:    make(map[string]profile.Record),
		sessions:    make(map[string]auth.Session),
	}
}

func (s *profileFlowStore) UpsertUserByFirebaseUID(_ context.Context, firebaseUID, email string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.userByUID[firebaseUID]; ok {
		existing.Email = email
		s.userByUID[firebaseUID] = existing
		return existing, false, nil
	}

	user := auth.User{
		ID:          profileFlowUserID,
		FirebaseUID: firebaseUID,
		Email:       email,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	s.userByUID[firebaseUID] = user
	s.userIDByUID[firebaseUID] = user.ID
	return user, true, nil
}

func (s *profileFlowStore) UpsertProfileOnRegister(_ context.Context, userID, displayName, preferredLanguage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.profiles[userID]; ok {
		return nil
	}
	s.profiles[userID] = profile.Record{
		Profile: auth.UserProfile{
			ID:                userID,
			FirebaseUID:       profileFlowFirebaseUID,
			DisplayName:       profileStrPtr(displayName),
			PreferredLanguage: preferredLanguage,
			CreatedAt:         time.Now().UTC(),
		},
	}
	return nil
}

func (s *profileFlowStore) GetUserProfileByUserID(ctx context.Context, userID string) (auth.UserProfile, error) {
	record, err := s.GetByUserID(ctx, userID)
	if err != nil {
		return auth.UserProfile{}, err
	}
	return record.Profile, nil
}

func (s *profileFlowStore) CreateSession(_ context.Context, session auth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *profileFlowStore) Revoke(_ context.Context, sessionID string, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.RevokedAt = &revokedAt
	s.sessions[sessionID] = session
	return nil
}

func (s *profileFlowStore) GetByID(_ context.Context, sessionID string) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	return session, nil
}

func (s *profileFlowStore) Touch(_ context.Context, sessionID string, lastActivityAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.LastActivityAt = lastActivityAt.UTC()
	s.sessions[sessionID] = session
	return nil
}

func (s *profileFlowStore) GetLocalUserIDByFirebaseUID(_ context.Context, firebaseUID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userIDByUID[firebaseUID]
	if !ok {
		return "", auth.ErrUserNotFound
	}
	return userID, nil
}

func (s *profileFlowStore) GetByUserID(_ context.Context, userID string) (profile.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.profiles[userID]
	if !ok {
		return profile.Record{}, auth.ErrUserNotFound
	}
	return record, nil
}

func (s *profileFlowStore) UpdateByUserID(_ context.Context, in profile.UpdateInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.profiles[in.UserID]
	if !ok {
		record = profile.Record{
			Profile: auth.UserProfile{
				ID:                in.UserID,
				FirebaseUID:       profileFlowFirebaseUID,
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		}
	}
	if in.FullName != nil {
		record.Profile.FullName = profileStrPtr(*in.FullName)
	}
	if in.DisplayName != nil {
		record.Profile.DisplayName = profileStrPtr(*in.DisplayName)
	}
	if in.Bio != nil {
		record.Profile.Bio = profileStrPtr(*in.Bio)
	}
	if in.Country != nil {
		record.Profile.Country = profileStrPtr(*in.Country)
	}
	if in.AvatarURL != nil {
		record.Profile.AvatarURL = profileStrPtr(*in.AvatarURL)
	}
	if in.PreferredLanguage != nil {
		record.Profile.PreferredLanguage = *in.PreferredLanguage
	}
	record.CompletedAt = in.CompletedAt
	s.profiles[in.UserID] = record
	return nil
}

func profileStrPtr(value string) *string {
	return &value
}
