package profile

import (
	"context"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestServiceUpdateMe_FirstCompletionValidation(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		record: Record{
			Profile: auth.UserProfile{
				ID:                "11111111-1111-1111-1111-111111111111",
				FirebaseUID:       "firebase-uid-1",
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		},
	}
	svc := NewService(store)

	_, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{
		DisplayName: strPtr("Ali"),
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Fields[httpconst.FieldFullName] == "" {
		t.Fatalf("expected %q validation field, got %v", httpconst.FieldFullName, validationErr.Fields)
	}
	if validationErr.Fields[httpconst.FieldCountry] == "" {
		t.Fatalf("expected %q validation field, got %v", httpconst.FieldCountry, validationErr.Fields)
	}
	if store.upsertCalls != 0 {
		t.Fatalf("expected no writes, got %d", store.upsertCalls)
	}
}

func TestServiceUpdateMe_CountryMustBeTwoLetterAlpha(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		record: Record{
			Profile: auth.UserProfile{
				ID:                "11111111-1111-1111-1111-111111111111",
				FirebaseUID:       "firebase-uid-1",
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		},
	}
	svc := NewService(store)

	_, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{
		FullName: strPtr("Ali Mahmoud"),
		Country:  strPtr("1A"),
	})
	if err == nil {
		t.Fatal("expected validation error")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if validationErr.Fields[httpconst.FieldCountry] == "" {
		t.Fatalf("expected %q validation field, got %v", httpconst.FieldCountry, validationErr.Fields)
	}
}

func TestServiceUpdateMe_FirstCompletionPersistsAndUppercasesCountry(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		record: Record{
			Profile: auth.UserProfile{
				ID:                "11111111-1111-1111-1111-111111111111",
				FirebaseUID:       "firebase-uid-1",
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		},
	}
	svc := NewService(store)
	fixedNow := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return fixedNow }

	updated, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{
		FullName: strPtr("Ali Mahmoud"),
		Country:  strPtr("eg"),
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Country == nil || *updated.Country != "EG" {
		t.Fatalf("expected uppercased country EG, got %+v", updated.Country)
	}
	if store.lastUpsert.CompletedAt == nil || !store.lastUpsert.CompletedAt.Equal(fixedNow) {
		t.Fatalf("expected completed_at %s, got %+v", fixedNow, store.lastUpsert.CompletedAt)
	}
}

func TestServiceUpdateMe_SubsequentUpdateDoesNotRequireFullNameCountry(t *testing.T) {
	t.Parallel()

	completedAt := time.Now().UTC().Add(-time.Hour)
	fullName := "Ali Mahmoud"
	country := "EG"
	displayName := "Ali"
	store := &stubStore{
		record: Record{
			Profile: auth.UserProfile{
				ID:                "11111111-1111-1111-1111-111111111111",
				FirebaseUID:       "firebase-uid-1",
				FullName:          &fullName,
				Country:           &country,
				DisplayName:       &displayName,
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
			CompletedAt: &completedAt,
		},
	}
	svc := NewService(store)

	updated, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{
		Bio: strPtr("Student"),
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Bio == nil || *updated.Bio != "Student" {
		t.Fatalf("expected bio to be updated, got %+v", updated.Bio)
	}
}

type stubStore struct {
	record      Record
	lastUpsert  UpdateInput
	upsertCalls int
}

func (s *stubStore) GetByUserID(_ context.Context, _ string) (Record, error) {
	return s.record, nil
}

func (s *stubStore) UpdateByUserID(_ context.Context, in UpdateInput) error {
	s.upsertCalls++
	s.lastUpsert = in
	if in.FullName != nil {
		s.record.Profile.FullName = strPtr(*in.FullName)
	}
	if in.DisplayName != nil {
		s.record.Profile.DisplayName = strPtr(*in.DisplayName)
	}
	if in.Bio != nil {
		s.record.Profile.Bio = strPtr(*in.Bio)
	}
	if in.Country != nil {
		s.record.Profile.Country = strPtr(*in.Country)
	}
	if in.AvatarURL != nil {
		s.record.Profile.AvatarURL = strPtr(*in.AvatarURL)
	}
	if in.PreferredLanguage != nil {
		s.record.Profile.PreferredLanguage = *in.PreferredLanguage
	}
	s.record.CompletedAt = in.CompletedAt
	return nil
}

func strPtr(value string) *string {
	return &value
}
