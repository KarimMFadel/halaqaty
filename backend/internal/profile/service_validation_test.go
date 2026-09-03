package profile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// completedRecord returns a store record whose first completion already happened,
// so field-level validation runs without the completion-required rules.
func completedRecord() Record {
	completedAt := time.Now().UTC().Add(-time.Hour)
	fullName := "Existing Name"
	country := "EG"
	return Record{
		Profile: auth.UserProfile{
			ID:                "11111111-1111-1111-1111-111111111113",
			FirebaseUID:       "firebase-validation-test",
			FullName:          &fullName,
			Country:           &country,
			PreferredLanguage: "ar",
			CreatedAt:         time.Now().UTC(),
		},
		CompletedAt: &completedAt,
	}
}

func TestServiceUpdateMe_EditableFieldValidation(t *testing.T) {
	t.Parallel()

	tooLong := func(n int) string { return strings.Repeat("x", n) }

	cases := []struct {
		name      string
		request   UpdateProfileRequest
		wantField string
		wantMsg   string
	}{
		{
			name:      "full name shorter than two runes is rejected",
			request:   UpdateProfileRequest{FullName: strPtr("A")},
			wantField: httpconst.FieldFullName,
			wantMsg:   httpconst.ErrorMessageFullNameInvalid,
		},
		{
			name:      "full name longer than 150 runes is rejected",
			request:   UpdateProfileRequest{FullName: strPtr(tooLong(151))},
			wantField: httpconst.FieldFullName,
			wantMsg:   httpconst.ErrorMessageFullNameInvalid,
		},
		{
			name:      "display name shorter than two runes is rejected",
			request:   UpdateProfileRequest{DisplayName: strPtr("A")},
			wantField: httpconst.FieldDisplayName,
			wantMsg:   httpconst.ErrorMessageDisplayNameInvalid,
		},
		{
			name:      "display name longer than 100 runes is rejected",
			request:   UpdateProfileRequest{DisplayName: strPtr(tooLong(101))},
			wantField: httpconst.FieldDisplayName,
			wantMsg:   httpconst.ErrorMessageDisplayNameInvalid,
		},
		{
			name:      "bio longer than 500 runes is rejected",
			request:   UpdateProfileRequest{Bio: strPtr(tooLong(501))},
			wantField: httpconst.FieldBio,
			wantMsg:   httpconst.ErrorMessageBioTooLong,
		},
		{
			name:      "country with three letters is rejected",
			request:   UpdateProfileRequest{Country: strPtr("EGY")},
			wantField: httpconst.FieldCountry,
			wantMsg:   httpconst.ErrorMessageCountryInvalid,
		},
		{
			name:      "unsupported preferred language is rejected",
			request:   UpdateProfileRequest{PreferredLanguage: strPtr("fr")},
			wantField: httpconst.FieldPreferredLanguage,
			wantMsg:   httpconst.ErrorMessagePreferredLanguageInvalid,
		},
		{
			name:      "avatar url that is not a uri is rejected",
			request:   UpdateProfileRequest{AvatarURL: strPtr("not a url")},
			wantField: httpconst.FieldAvatarURL,
			wantMsg:   httpconst.ErrorMessageAvatarURLInvalid,
		},
		{
			name:      "phone longer than 50 runes is rejected",
			request:   UpdateProfileRequest{Phone: strPtr(tooLong(51))},
			wantField: httpconst.FieldPhone,
			wantMsg:   httpconst.ErrorMessagePhoneTooLong,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &validationStore{record: completedRecord()}
			svc := NewService(store)

			_, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, tc.request)
			if err == nil {
				t.Fatal("expected validation error")
			}
			validationErr := &ValidationError{}
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if got := validationErr.Fields[tc.wantField]; got != tc.wantMsg {
				t.Fatalf("field %q: got %q, want %q fields=%v", tc.wantField, got, tc.wantMsg, validationErr.Fields)
			}
			if store.updates != 0 {
				t.Fatalf("invalid input must not persist, got %d updates", store.updates)
			}
		})
	}
}

func TestServiceUpdateMe_AcceptsEdgeCaseFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		request UpdateProfileRequest
	}{
		{
			name:    "empty avatar url is accepted to clear the avatar",
			request: UpdateProfileRequest{AvatarURL: strPtr("")},
		},
		{
			name:    "phone at the maximum length is accepted",
			request: UpdateProfileRequest{Phone: strPtr(strings.Repeat("1", 50))},
		},
		{
			name:    "arabic language preference is accepted",
			request: UpdateProfileRequest{PreferredLanguage: strPtr("en")},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &validationStore{record: completedRecord()}
			svc := NewService(store)

			if _, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, tc.request); err != nil {
				t.Fatalf("update failed: %v", err)
			}
			if store.updates != 1 {
				t.Fatalf("updates: got %d, want 1", store.updates)
			}
		})
	}
}

func TestServiceUpdateMe_LoadFailureWrapsError(t *testing.T) {
	t.Parallel()
	store := &validationStore{record: completedRecord(), failGet: true}
	svc := NewService(store)

	_, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{Bio: strPtr("bio")})
	if err == nil {
		t.Fatal("expected load error")
	}
	if !strings.Contains(err.Error(), "load profile before update") {
		t.Fatalf("error should carry load context, got %q", err.Error())
	}
	if !errors.Is(err, errValidationStoreGet) {
		t.Fatalf("wrapped store error must survive wrapping, got %v", err)
	}
}

func TestServiceUpdateMe_PersistFailureWrapsError(t *testing.T) {
	t.Parallel()
	store := &validationStore{record: completedRecord(), failUpdate: true}
	svc := NewService(store)

	_, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{Bio: strPtr("bio")})
	if err == nil {
		t.Fatal("expected persist error")
	}
	if !strings.Contains(err.Error(), "update profile") {
		t.Fatalf("error should carry update context, got %q", err.Error())
	}
	if !errors.Is(err, errValidationStoreUpdate) {
		t.Fatalf("wrapped store error must survive wrapping, got %v", err)
	}
}

func TestServiceUpdateMe_CompletedAtIsPreservedWhenNotFirstCompletion(t *testing.T) {
	t.Parallel()
	store := &validationStore{record: completedRecord()}
	svc := NewService(store)
	svc.nowFn = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }

	if _, err := svc.UpdateMe(context.Background(), store.record.Profile.ID, UpdateProfileRequest{Bio: strPtr("bio")}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !store.lastInput.CompletedAt.Equal(*store.record.CompletedAt) {
		t.Fatalf("completed_at: got %+v, want the pre-existing value %+v", store.lastInput.CompletedAt, store.record.CompletedAt)
	}
}

func TestValidationError_Error_ReturnsStableMessage(t *testing.T) {
	t.Parallel()

	err := &ValidationError{Fields: map[string]string{httpconst.FieldBio: httpconst.ErrorMessageBioTooLong}}
	if err.Error() != httpconst.ErrorMessageValidationFailed {
		t.Fatalf("Error(): got %q, want %q", err.Error(), httpconst.ErrorMessageValidationFailed)
	}
}

var (
	errValidationStoreGet    = errors.New("validation store get failed")
	errValidationStoreUpdate = errors.New("validation store update failed")
)

type validationStore struct {
	record     Record
	failGet    bool
	failUpdate bool
	updates    int
	lastInput  UpdateInput
}

func (s *validationStore) GetByUserID(context.Context, string) (Record, error) {
	if s.failGet {
		return Record{}, errValidationStoreGet
	}
	return s.record, nil
}

func (s *validationStore) UpdateByUserID(_ context.Context, in UpdateInput) error {
	s.updates++
	s.lastInput = in
	if s.failUpdate {
		return errValidationStoreUpdate
	}
	if in.Bio != nil {
		s.record.Profile.Bio = strPtr(*in.Bio)
	}
	if in.AvatarURL != nil {
		s.record.Profile.AvatarURL = strPtr(*in.AvatarURL)
	}
	if in.Phone != nil {
		s.record.Profile.Phone = strPtr(*in.Phone)
	}
	if in.PreferredLanguage != nil {
		s.record.Profile.PreferredLanguage = *in.PreferredLanguage
	}
	s.record.CompletedAt = in.CompletedAt
	return nil
}
