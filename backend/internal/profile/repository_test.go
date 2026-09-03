package profile

import (
	"database/sql"
	"testing"
	"time"
)

func TestNullHelpers_ConvertValues(t *testing.T) {
	t.Parallel()

	validString := sql.NullString{String: "Ali", Valid: true}
	invalidString := sql.NullString{}
	fixedTime := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	validTime := sql.NullTime{Time: fixedTime, Valid: true}
	invalidTime := sql.NullTime{}
	value := "EG"

	if got := nullStringPtr(validString); got == nil || *got != "Ali" {
		t.Fatalf("nullStringPtr(valid): got %+v, want %q", got, "Ali")
	}
	if got := nullStringPtr(invalidString); got != nil {
		t.Fatalf("nullStringPtr(invalid): got %+v, want nil", got)
	}
	if got := nullTimePtr(validTime); got == nil || !got.Equal(fixedTime) {
		t.Fatalf("nullTimePtr(valid): got %+v, want %s", got, fixedTime)
	}
	if got := nullTimePtr(invalidTime); got != nil {
		t.Fatalf("nullTimePtr(invalid): got %+v, want nil", got)
	}
	if got := derefOrNil(&value); got != "EG" {
		t.Fatalf("derefOrNil(ptr): got %v, want %q", got, "EG")
	}
	if got := derefOrNil(nil); got != nil {
		t.Fatalf("derefOrNil(nil): got %v, want nil", got)
	}
}
