package chat

import "testing"

func TestQualifiesDirectRolePair(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{name: "teacher student", left: "teacher", right: "student", want: true},
		{name: "student teacher", left: "student", right: "teacher", want: true},
		{name: "supervisor student", left: "supervisor", right: "student", want: true},
		{name: "student supervisor", left: "student", right: "supervisor", want: true},
		{name: "student student", left: "student", right: "student"},
		{name: "teacher teacher", left: "teacher", right: "teacher"},
		{name: "supervisor supervisor", left: "supervisor", right: "supervisor"},
		{name: "teacher supervisor", left: "teacher", right: "supervisor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := qualifiesDirectRolePair(tt.left, tt.right); got != tt.want {
				t.Fatalf("qualifiesDirectRolePair(%q, %q) = %v, want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}
