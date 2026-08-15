//go:build contract

package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleCreateContract_SettingsAndLimits(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
		check      func(t *testing.T, response rbac.CircleResponse)
	}{
		{"name at maximum length", `{"name":"` + strings.Repeat("a", 100) + `"}`, http.StatusCreated, nil},
		{"name over maximum", `{"name":"` + strings.Repeat("a", 101) + `"}`, http.StatusBadRequest, nil},
		{"description at maximum", `{"name":"Circle","description":"` + strings.Repeat("d", 500) + `"}`, http.StatusCreated, nil},
		{"description over maximum", `{"name":"Circle","description":"` + strings.Repeat("d", 501) + `"}`, http.StatusBadRequest, nil},
		{"rules at maximum", `{"name":"Circle","rules":"` + strings.Repeat("r", 1000) + `"}`, http.StatusCreated, nil},
		{"rules over maximum", `{"name":"Circle","rules":"` + strings.Repeat("r", 1001) + `"}`, http.StatusBadRequest, nil},
		{"capacity minimum", `{"name":"Circle","max_capacity":2}`, http.StatusCreated, nil},
		{"capacity maximum", `{"name":"Circle","max_capacity":200}`, http.StatusCreated, nil},
		{"capacity below minimum", `{"name":"Circle","max_capacity":1}`, http.StatusBadRequest, nil},
		{"capacity above maximum", `{"name":"Circle","max_capacity":201}`, http.StatusBadRequest, nil},
		{"custom language", `{"name":"Circle","language":"fr"}`, http.StatusCreated, func(t *testing.T, response rbac.CircleResponse) {
			if response.Language != "fr" {
				t.Fatalf("language: got %q", response.Language)
			}
		}},
		{"language over maximum", `{"name":"Circle","language":"` + strings.Repeat("l", 11) + `"}`, http.StatusBadRequest, nil},
		{"defaults and invite link", `{"name":"Circle"}`, http.StatusCreated, func(t *testing.T, response rbac.CircleResponse) {
			if response.MaxCapacity != 50 || response.GenderRestriction != "unspecified" || response.Language != "ar" {
				t.Fatalf("defaults: %+v", response)
			}
			if response.InviteLink != "https://halaqaty.app/join/"+response.InviteCode {
				t.Fatalf("invite link: %q", response.InviteLink)
			}
		}},
		{"private circle", `{"name":"Circle","is_private":true}`, http.StatusCreated, func(t *testing.T, response rbac.CircleResponse) {
			if !response.IsPrivate {
				t.Fatal("expected private circle")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			req := httptest.NewRequest(http.MethodPost, "/circles", bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()
			buildCreateCircleRoute(store).ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.check != nil {
				var response rbac.CircleResponse
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				tc.check(t, response)
			}
		})
	}
}

func TestCircleCreateContract_AllGenderValues(t *testing.T) {
	for _, gender := range []string{"male", "female", "mixed", "unspecified"} {
		t.Run(gender, func(t *testing.T) {
			store := newCircleStoreStub()
			req := httptest.NewRequest(http.MethodPost, "/circles", strings.NewReader(`{"name":"Circle","gender_restriction":"`+gender+`"}`))
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()
			buildCreateCircleRoute(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	store := newCircleStoreStub()
	req := httptest.NewRequest(http.MethodPost, "/circles", strings.NewReader(`{"name":"Circle","gender_restriction":"other"}`))
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	rec := httptest.NewRecorder()
	buildCreateCircleRoute(store).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid gender status: got %d", rec.Code)
	}
}
