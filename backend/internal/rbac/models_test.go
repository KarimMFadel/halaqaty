package rbac

import (
	"encoding/json"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestNullableStringUpdate_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	mosque := "mosque hall"
	cases := []struct {
		name      string
		body      string
		wantSet   bool
		wantValue *string
		wantErr   bool
	}{
		{name: "omitted field is not marked set", body: `{}`},
		{name: "explicit null marks set with nil value", body: `{"description":null}`, wantSet: true},
		{name: "string value marks set with value", body: `{"description":"mosque hall"}`, wantSet: true, wantValue: &mosque},
		{name: "non-string value is rejected", body: `{"description":42}`, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var req UpdateCircleRequest
			err := json.Unmarshal([]byte(tc.body), &req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected decode error, got %+v", req.Description)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if req.Description.Set != tc.wantSet {
				t.Fatalf("Set: got %v want %v", req.Description.Set, tc.wantSet)
			}
			switch {
			case tc.wantValue == nil && req.Description.Value != nil:
				t.Fatalf("Value: got %q want nil", *req.Description.Value)
			case tc.wantValue != nil && req.Description.Value == nil:
				t.Fatalf("Value: got nil want %q", *tc.wantValue)
			case tc.wantValue != nil && *req.Description.Value != *tc.wantValue:
				t.Fatalf("Value: got %q want %q", *req.Description.Value, *tc.wantValue)
			}
		})
	}
}

func TestValidationError_Error_ReturnsStableTopLevelMessage(t *testing.T) {
	t.Parallel()

	err := &ValidationError{Fields: map[string]string{httpconst.FieldName: httpconst.ErrorMessageCircleNameInvalid}}
	if err.Error() != httpconst.ErrorMessageValidationFailed {
		t.Fatalf("Error(): got %q want %q", err.Error(), httpconst.ErrorMessageValidationFailed)
	}
}

func intPtr(value int) *int { return &value }

func TestMergeCircleUpdate(t *testing.T) {
	t.Parallel()

	base := Circle{
		Name: "Base Circle", MaxCapacity: 30, IsPrivate: true,
		GenderRestriction: "male", Language: "en", GradingPolicy: "optional",
	}
	newName := "Renamed Hall"
	description := "new description"
	rules := "new rules"
	capacity := 100
	notPrivate := false
	gender := "mixed"
	language := "en"
	grading := "optional"

	cases := []struct {
		name       string
		circle     Circle
		req        UpdateCircleRequest
		wantName   string
		assertSet  func(*testing.T, CircleSettings)
		wantFields []string
	}{
		{
			name:     "empty request preserves existing settings",
			circle:   base,
			req:      UpdateCircleRequest{},
			wantName: "Base Circle",
			assertSet: func(t *testing.T, s CircleSettings) {
				t.Helper()
				if s.MaxCapacity != 30 || !s.IsPrivate || s.GenderRestriction != "male" ||
					s.Language != "en" || s.GradingPolicy != "optional" {
					t.Fatalf("empty update mutated settings: %+v", s)
				}
			},
		},
		{
			name:     "zero circle settings fall back to MVP defaults",
			circle:   Circle{Name: "Blank Circle"},
			req:      UpdateCircleRequest{},
			wantName: "Blank Circle",
			assertSet: func(t *testing.T, s CircleSettings) {
				t.Helper()
				if s.MaxCapacity != 50 || s.IsPrivate || s.GenderRestriction != "unspecified" ||
					s.Language != "ar" || s.GradingPolicy != "required" {
					t.Fatalf("expected MVP defaults, got %+v", s)
				}
			},
		},
		{
			name:   "provided fields replace stored values",
			circle: base,
			req: UpdateCircleRequest{
				Name:              &newName,
				Description:       NullableStringUpdate{Set: true, Value: &description},
				Rules:             NullableStringUpdate{Set: true, Value: &rules},
				MaxCapacity:       &capacity,
				IsPrivate:         &notPrivate,
				GenderRestriction: &gender,
				Language:          &language,
				GradingPolicy:     &grading,
			},
			wantName: newName,
			assertSet: func(t *testing.T, s CircleSettings) {
				t.Helper()
				if s.Description == nil || *s.Description != description {
					t.Fatalf("description: got %v want %q", s.Description, description)
				}
				if s.Rules == nil || *s.Rules != rules {
					t.Fatalf("rules: got %v want %q", s.Rules, rules)
				}
				if s.MaxCapacity != capacity || s.IsPrivate {
					t.Fatalf("capacity/privacy not applied: %+v", s)
				}
				if s.GenderRestriction != gender || s.Language != language || s.GradingPolicy != grading {
					t.Fatalf("enum fields not applied: %+v", s)
				}
			},
		},
		{
			name:       "below-range capacity is rejected",
			circle:     base,
			req:        UpdateCircleRequest{MaxCapacity: intPtr(1)},
			wantFields: []string{httpconst.FieldMaxCapacity},
		},
		{
			name:       "blank name is rejected",
			circle:     base,
			req:        UpdateCircleRequest{Name: strPtr("   ")},
			wantFields: []string{httpconst.FieldName},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, settings, fields := mergeCircleUpdate(tc.circle, tc.req)

			for _, field := range tc.wantFields {
				if _, ok := fields[field]; !ok {
					t.Fatalf("expected field %q in %v", field, fields)
				}
			}
			if len(tc.wantFields) == 0 && len(fields) > 0 {
				t.Fatalf("unexpected validation fields: %v", fields)
			}
			if tc.assertSet != nil {
				if name != tc.wantName {
					t.Fatalf("name: got %q want %q", name, tc.wantName)
				}
				tc.assertSet(t, settings)
			}
		})
	}
}
