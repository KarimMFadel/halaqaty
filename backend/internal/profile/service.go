package profile

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

const (
	fullNameMinLen    = 2
	fullNameMaxLen    = 150
	displayNameMinLen = 2
	displayNameMaxLen = 100
	bioMaxLen         = 500
	phoneMaxLen       = 50
)

var countryCodePattern = regexp.MustCompile(`^[A-Za-z]{2}$`)
var profileLanguages = map[string]struct{}{"ar": {}, "en": {}}

// Store is the profile persistence contract.
type Store interface {
	GetByUserID(ctx context.Context, userID string) (Record, error)
	UpdateByUserID(ctx context.Context, in UpdateInput) error
}

// UpdateProfileRequest is the payload for PUT /auth/me.
type UpdateProfileRequest struct {
	FullName          *string `json:"full_name"`
	DisplayName       *string `json:"display_name"`
	Bio               *string `json:"bio"`
	Country           *string `json:"country"`
	PreferredLanguage *string `json:"preferred_language"`
	AvatarURL         *string `json:"avatar_url"`
	Phone             *string `json:"phone"`
}

// ValidationError carries field-level profile validation failures.
type ValidationError struct {
	Fields map[string]string
}

// Error returns a stable top-level validation message.
func (e *ValidationError) Error() string {
	return httpconst.ErrorMessageValidationFailed
}

// Service groups profile use cases.
type Service struct {
	store Store
	nowFn func() time.Time
}

// NewService constructs a profile service.
func NewService(store Store) *Service {
	return &Service{
		store: store,
		nowFn: time.Now,
	}
}

// GetMe reads the authenticated caller's profile.
func (s *Service) GetMe(ctx context.Context, userID string) (auth.UserProfile, error) {
	record, err := s.store.GetByUserID(ctx, userID)
	if err != nil {
		return auth.UserProfile{}, fmt.Errorf("get profile: %w", err)
	}
	return record.Profile, nil
}

// UpdateMe updates editable profile fields for the authenticated caller.
func (s *Service) UpdateMe(ctx context.Context, userID string, req UpdateProfileRequest) (auth.UserProfile, error) {
	current, err := s.store.GetByUserID(ctx, userID)
	if err != nil {
		return auth.UserProfile{}, fmt.Errorf("load profile before update: %w", err)
	}

	next := UpdateInput{
		UserID:      userID,
		CompletedAt: current.CompletedAt,
	}

	fields := make(map[string]string)

	if req.FullName != nil {
		fullName := strings.TrimSpace(*req.FullName)
		if length := utf8.RuneCountInString(fullName); length < fullNameMinLen || length > fullNameMaxLen {
			fields[httpconst.FieldFullName] = httpconst.ErrorMessageFullNameInvalid
		} else {
			next.FullName = &fullName
		}
	}

	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		if length := utf8.RuneCountInString(displayName); length < displayNameMinLen || length > displayNameMaxLen {
			fields[httpconst.FieldDisplayName] = httpconst.ErrorMessageDisplayNameInvalid
		} else {
			next.DisplayName = &displayName
		}
	}

	if req.Bio != nil {
		bio := strings.TrimSpace(*req.Bio)
		if utf8.RuneCountInString(bio) > bioMaxLen {
			fields[httpconst.FieldBio] = httpconst.ErrorMessageBioTooLong
		} else {
			next.Bio = &bio
		}
	}

	if req.Country != nil {
		country := strings.ToUpper(strings.TrimSpace(*req.Country))
		if !countryCodePattern.MatchString(country) {
			fields[httpconst.FieldCountry] = httpconst.ErrorMessageCountryInvalid
		} else {
			next.Country = &country
		}
	}

	if req.PreferredLanguage != nil {
		language := strings.TrimSpace(*req.PreferredLanguage)
		if _, ok := profileLanguages[language]; !ok {
			fields[httpconst.FieldPreferredLanguage] = httpconst.ErrorMessagePreferredLanguageInvalid
		} else {
			next.PreferredLanguage = &language
		}
	}

	if req.AvatarURL != nil {
		avatarURL := strings.TrimSpace(*req.AvatarURL)
		if avatarURL != "" {
			if _, parseErr := url.ParseRequestURI(avatarURL); parseErr != nil {
				fields[httpconst.FieldAvatarURL] = httpconst.ErrorMessageAvatarURLInvalid
			} else {
				next.AvatarURL = &avatarURL
			}
		} else {
			next.AvatarURL = &avatarURL
		}
	}

	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if utf8.RuneCountInString(phone) > phoneMaxLen {
			fields[httpconst.FieldPhone] = httpconst.ErrorMessagePhoneTooLong
		} else {
			next.Phone = &phone
		}
	}

	firstCompletion := current.CompletedAt == nil
	if firstCompletion {
		if _, hasError := fields[httpconst.FieldFullName]; !hasError && (next.FullName == nil || strings.TrimSpace(*next.FullName) == "") {
			fields[httpconst.FieldFullName] = httpconst.ErrorMessageFullNameRequired
		}
		if _, hasError := fields[httpconst.FieldCountry]; !hasError && (next.Country == nil || strings.TrimSpace(*next.Country) == "") {
			fields[httpconst.FieldCountry] = httpconst.ErrorMessageCountryRequired
		}
	}

	if len(fields) > 0 {
		return auth.UserProfile{}, &ValidationError{Fields: fields}
	}

	if firstCompletion {
		completedAt := s.nowFn().UTC().Truncate(time.Second)
		next.CompletedAt = &completedAt
	}

	if err := s.store.UpdateByUserID(ctx, next); err != nil {
		return auth.UserProfile{}, fmt.Errorf("update profile: %w", err)
	}
	return s.GetMe(ctx, userID)
}
