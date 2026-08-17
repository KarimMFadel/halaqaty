package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CircleTopicReader supplies the caller's current active circle memberships.
type CircleTopicReader interface {
	ListCircleIDs(context.Context, string) ([]string, error)
}

// TicketService issues short-lived in-memory realtime tickets. Tickets carry
// only circle authorization and never media credentials or room references.
type TicketService struct {
	reader CircleTopicReader
	now    func() time.Time
	mu     sync.Mutex
	tokens map[string]Ticket
}

// NewTicketService constructs the generic realtime ticket service.
func NewTicketService(reader CircleTopicReader) *TicketService {
	return &TicketService{reader: reader, now: time.Now, tokens: map[string]Ticket{}}
}

// Issue creates a 60-second ticket for the user's current circles.
func (s *TicketService) Issue(ctx context.Context, userID string) (Ticket, error) {
	if s == nil || s.reader == nil {
		return Ticket{}, errors.New("realtime ticket service is not configured")
	}
	if userID == "" {
		return Ticket{}, errors.New("realtime ticket user is required")
	}
	circles, err := s.reader.ListCircleIDs(ctx, userID)
	if err != nil {
		return Ticket{}, fmt.Errorf("list realtime circles: %w", err)
	}
	ticket := Ticket{Token: uuid.NewString(), UserID: userID, CircleIDs: circles, ExpiresAt: s.now().UTC().Add(TicketTTL)}
	s.mu.Lock()
	s.tokens[ticket.Token] = ticket
	s.mu.Unlock()
	return ticket, nil
}

// Validate returns a live ticket and rejects unknown, expired, or mismatched tokens.
func (s *TicketService) Validate(token, userID string) (Ticket, error) {
	s.mu.Lock()
	ticket, ok := s.tokens[token]
	if ok && ticket.ExpiredAt(s.now()) {
		delete(s.tokens, token)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return Ticket{}, errors.New("realtime ticket is invalid or expired")
	}
	if userID != "" && ticket.UserID != userID {
		return Ticket{}, errors.New("realtime ticket user mismatch")
	}
	return ticket, nil
}
