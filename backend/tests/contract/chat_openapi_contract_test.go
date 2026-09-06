//go:build contract

package contract

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

type chatOpenAPIOperation struct {
	block     string
	responses map[string]bool
}

var chatOpenAPIOperationPattern = regexp.MustCompile(`(?m)^[ \t]+operationId:[ \t]+(\S+)[ \t]*\r?$`)
var chatOpenAPIResponsePattern = regexp.MustCompile(`(?m)^[ \t]*'([0-9]{3})':`)

func TestChatOpenAPIContract_Parity(t *testing.T) {
	canonical := readChatOpenAPI(t, "../../../docs/contracts/openapi.yaml")
	feature := readChatOpenAPI(t, "../../../specs/004-real-time-chat/contracts/chat.openapi.yaml")

	canonicalOperations := parseChatOpenAPIOperations(canonical)
	featureOperations := parseChatOpenAPIOperations(feature)
	for _, operationID := range chatOperationIDs {
		canonicalOperation, canonicalOK := canonicalOperations[operationID]
		featureOperation, featureOK := featureOperations[operationID]
		if !canonicalOK || !featureOK {
			t.Errorf("operationId %q must exist in canonical and feature contracts (canonical=%t feature=%t)", operationID, canonicalOK, featureOK)
			continue
		}
		for responseCode := range featureOperation.responses {
			if !canonicalOperation.responses[responseCode] {
				t.Errorf("%s response code %s is missing from canonical contract", operationID, responseCode)
			}
		}
	}

	assertChatSchemaParity(t, canonical, feature)
	assertChatContractInvariant(t, canonicalOperations, parseChatOpenAPISchemaBlocks(canonical), "canonical")
	assertChatContractInvariant(t, featureOperations, parseChatOpenAPISchemaBlocks(feature), "feature")
}

var chatOperationIDs = []string{
	"listCircleMessages", "sendCircleMessage", "searchCircleMessages", "listPinnedCircleMessages",
	"deleteCircleMessage", "pinCircleMessage", "unpinCircleMessage", "markCircleMessageRead",
	"listDirectMessages", "sendDirectMessage", "deleteDirectMessage", "markDirectMessageRead",
	"renewMessageMediaUrl", "uploadVoice", "uploadImage", "uploadFile",
}

func readChatOpenAPI(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI document %q: %v", path, err)
	}
	return string(contents)
}

func parseChatOpenAPIOperations(document string) map[string]chatOpenAPIOperation {
	operations := make(map[string]chatOpenAPIOperation)
	matches := chatOpenAPIOperationPattern.FindAllStringSubmatchIndex(document, -1)
	for index, match := range matches {
		end := len(document)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		operationID := document[match[2]:match[3]]
		block := document[match[1]:end]
		responses := make(map[string]bool)
		for _, response := range chatOpenAPIResponsePattern.FindAllStringSubmatch(block, -1) {
			responses[response[1]] = true
		}
		operations[operationID] = chatOpenAPIOperation{block: block, responses: responses}
	}
	return operations
}

func assertChatSchemaParity(t *testing.T, canonical, feature string) {
	t.Helper()
	canonicalSchemas := parseChatOpenAPISchemas(canonical)
	featureSchemas := parseChatOpenAPISchemas(feature)
	for _, schemaName := range chatSchemaNames {
		canonicalProperties, canonicalOK := canonicalSchemas[schemaName]
		featureProperties, featureOK := featureSchemas[schemaName]
		if !canonicalOK || !featureOK {
			t.Errorf("schema %q must exist in canonical and feature contracts (canonical=%t feature=%t)", schemaName, canonicalOK, featureOK)
			continue
		}
		for property := range featureProperties {
			if !canonicalProperties[property] {
				t.Errorf("feature property %q is missing from canonical schema %q", property, schemaName)
			}
		}
	}
}

var chatSchemaNames = []string{
	"SendMessageRequest", "Message", "ReadReceipt", "ReplyPreview", "PaginatedMessages", "UploadResponse", "MediaAccess", "ErrorResponse",
}

func parseChatOpenAPISchemas(document string) map[string]map[string]bool {
	schemas := make(map[string]map[string]bool)
	for name, block := range parseChatOpenAPISchemaBlocks(document) {
		properties := make(map[string]bool)
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "        ") && !strings.HasPrefix(line, "         ") {
				property := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
				if property != "" {
					properties[property] = true
				}
			}
		}
		schemas[name] = properties
	}
	return schemas
}

func parseChatOpenAPISchemaBlocks(document string) map[string]string {
	schemas := make(map[string]string)
	lines := strings.Split(strings.ReplaceAll(document, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if !isChatSchemaHeader(line) {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSpace(line), ":")
		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			if isChatSchemaHeader(lines[next]) {
				end = next
				break
			}
		}
		schemas[name] = strings.Join(lines[index:end], "\n")
	}
	return schemas
}

func isChatSchemaHeader(line string) bool {
	return strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") && strings.HasSuffix(strings.TrimSpace(line), ":")
}

func assertChatContractInvariant(t *testing.T, operations map[string]chatOpenAPIOperation, schemas map[string]string, documentName string) {
	t.Helper()
	assertChatOperationContainsAny(t, operations, "listPinnedCircleMessages", []string{"PinnedMessages", "maxItems: 5"}, documentName)
	assertChatOperationContains(t, operations, "markCircleMessageRead", "active circle", documentName)
	assertChatOperationContains(t, operations, "markCircleMessageRead", "archived-circle history is read-only", documentName)
	for _, operationID := range []string{"uploadVoice", "uploadImage", "uploadFile"} {
		operation := operations[operationID]
		if !operation.responses["415"] {
			t.Errorf("%s %s must declare 415", documentName, operationID)
		}
	}

	message := schemas["Message"]
	for _, want := range []string{
		"Present only when the caller is the sender",
		"enum: [delivered, read]",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("%s Message schema must contain %q", documentName, want)
		}
	}
	for _, forbidden := range []string{"enum: [pending, sent]", "enum: [sent, delivered, read]"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("%s Message schema must not make %q a server delivery state", documentName, forbidden)
		}
	}
}

func assertChatOperationContains(t *testing.T, operations map[string]chatOpenAPIOperation, operationID, want, documentName string) {
	t.Helper()
	operation, ok := operations[operationID]
	if !ok || !strings.Contains(operation.block, want) {
		t.Errorf("%s %s must contain %q", documentName, operationID, want)
	}
}

func assertChatOperationContainsAny(t *testing.T, operations map[string]chatOpenAPIOperation, operationID string, wants []string, documentName string) {
	t.Helper()
	operation, ok := operations[operationID]
	if !ok {
		t.Errorf("%s missing operation %q", documentName, operationID)
		return
	}
	for _, want := range wants {
		if strings.Contains(operation.block, want) {
			return
		}
	}
	t.Errorf("%s %s must contain one of %q", documentName, operationID, wants)
}
