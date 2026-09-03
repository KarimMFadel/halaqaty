package httpconst

import "testing"

func TestIsJSONContentType_AcceptsJSONMediaTypes(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"exact application/json", "application/json", true},
		{"json with charset parameter", "application/json; charset=utf-8", true},
		{"whitespace padded", "  application/json  ", true},
		{"case insensitive type", "Application/JSON", true},
		{"empty header", "", false},
		{"whitespace only", "   ", false},
		{"non-json type", "text/plain", false},
		{"json-prefixed subtype is not json", "application/json-patch+json", false},
		{"invalid parameter value", "application/json; charset==bad", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsJSONContentType(tc.header); got != tc.want {
				t.Fatalf("IsJSONContentType(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}
