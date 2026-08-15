//go:build contract

package contract

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestPublicCircleSummaryResponseSafety(t *testing.T) {
	secret := "HLQ-7X2K"
	body, err := json.Marshal(rbac.PublicCircleSummary{
		ID: "circle-1", Name: "Public Circle", MaxCapacity: 50,
		GenderRestriction: "unspecified", Language: "ar",
	})
	if err != nil {
		t.Fatalf("marshal public summary: %v", err)
	}
	text := strings.ToLower(string(body))
	for _, forbidden := range []string{secret, "invite_code", "invite_link", "user_id", "role", "is_private"} {
		if strings.Contains(text, strings.ToLower(forbidden)) {
			t.Fatalf("public summary leaked %q: %s", forbidden, body)
		}
	}
}
