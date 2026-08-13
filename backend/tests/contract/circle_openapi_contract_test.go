//go:build contract

package contract

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	openAPIOperationPattern = regexp.MustCompile(`(?m)^\s+operationId:\s+(\S+)\s*$`)
	openAPIRefPattern       = regexp.MustCompile(`\$ref:\s*['"]?#/components/([^'"}\s]+)`)
)

func TestCircleOpenAPIContract_ReferencesOperationsAndSecurity(t *testing.T) {
	documents := []string{
		"../../../docs/contracts/openapi.yaml",
		"../../../specs/002-circle-management/contracts/circle-management.openapi.yaml",
	}
	for _, path := range documents {
		t.Run(path, func(t *testing.T) {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read OpenAPI document: %v", err)
			}
			text := string(contents)
			if !strings.Contains(text, "BearerAuth: []") || !strings.Contains(text, "SessionId: []") {
				t.Fatal("circle contract must require bearer and backend-session security")
			}

			seen := map[string]bool{}
			for _, match := range openAPIOperationPattern.FindAllStringSubmatch(text, -1) {
				if seen[match[1]] {
					t.Fatalf("duplicate operationId %q", match[1])
				}
				seen[match[1]] = true
			}
			for _, operationID := range circleOperationIDs {
				if !seen[operationID] {
					t.Errorf("missing operationId %q", operationID)
				}
			}
			for _, match := range openAPIRefPattern.FindAllStringSubmatch(text, -1) {
				parts := strings.Split(match[1], "/")
				name := parts[len(parts)-1]
				if !strings.Contains(text, "    "+name+":") {
					t.Errorf("unresolved local reference %q", match[0])
				}
			}
		})
	}
}

var circleOperationIDs = []string{
	"listCircles",
	"createCircle",
	"searchUsers",
	"getCircle",
	"updateCircle",
	"archiveCircle",
	"joinPublicCircle",
	"discoverPublicCircles",
	"listCircleMembers",
	"removeCircleMember",
	"updateCircleMemberRole",
	"joinCircle",
	"refreshCircleInviteCode",
}
