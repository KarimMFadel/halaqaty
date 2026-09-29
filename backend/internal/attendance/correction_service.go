package attendance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrAttendanceForbidden   = errors.New("attendance access denied")
	ErrAttendanceNotFound    = errors.New("attendance not found")
	ErrAttendanceArchived    = errors.New("archived circle attendance is read-only")
	ErrAttendanceNotComplete = errors.New("attendance is not finalized")
	ErrInvalidCorrection     = errors.New("invalid attendance correction")
	ErrCorrectionReplay      = errors.New("idempotency key reused for different attendance correction")
)

// CorrectionCommand is one teacher's audited attendance override.
type CorrectionCommand struct {
	ActorID        string
	SessionID      string
	UserID         string
	IdempotencyKey string
	Status         Status
	Reason         string
}

// CorrectionAudit identifies the latest append-only override, when present.
type CorrectionAudit struct {
	ActorID        string    `json:"actor_id"`
	At             time.Time `json:"at"`
	Reason         string    `json:"reason"`
	PreviousStatus Status    `json:"previous_status"`
	NewStatus      Status    `json:"new_status"`
}

// Record is the role-filtered API projection of one student's attendance.
type Record struct {
	SessionID     string           `json:"session_id"`
	UserID        string           `json:"user_id"`
	Status        Status           `json:"status"`
	Source        string           `json:"source"`
	FirstPresence *time.Time       `json:"first_presence_at"`
	CorrectionID  *string          `json:"correction_id"`
	Correction    *CorrectionAudit `json:"correction"`
}

// CorrectionService validates teacher correction commands before persistence.
type CorrectionService struct {
	repository *Repository
}

// NewCorrectionService constructs the attendance correction service.
func NewCorrectionService(repository *Repository) *CorrectionService {
	return &CorrectionService{repository: repository}
}

// List returns attendance visible to the authenticated member.
func (s *CorrectionService) List(ctx context.Context, sessionID, actorID string) ([]Record, error) {
	return s.repository.List(ctx, sessionID, actorID)
}

// Correct appends an audited correction and returns its effective attendance.
func (s *CorrectionService) Correct(ctx context.Context, command CorrectionCommand) (Record, error) {
	if err := validateCorrection(command); err != nil {
		return Record{}, err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return Record{}, fmt.Errorf("fingerprint attendance correction: %w", err)
	}
	fingerprint := sha256.Sum256(payload)
	return s.repository.CorrectAttendance(ctx, command, hex.EncodeToString(fingerprint[:]))
}

func validateCorrection(command CorrectionCommand) error {
	if command.ActorID == "" || command.SessionID == "" || command.UserID == "" || command.IdempotencyKey == "" || len(command.IdempotencyKey) > 128 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return fmt.Errorf("missing or invalid correction identity: %w", ErrInvalidCorrection)
	}
	switch command.Status {
	case StatusPresent, StatusLate, StatusAbsent, StatusExcused:
	default:
		return fmt.Errorf("unknown attendance status: %w", ErrInvalidCorrection)
	}
	if command.Reason == "" || strings.TrimSpace(command.Reason) != command.Reason || utf8.RuneCountInString(command.Reason) > 1000 {
		return fmt.Errorf("missing or invalid correction reason: %w", ErrInvalidCorrection)
	}
	return nil
}
