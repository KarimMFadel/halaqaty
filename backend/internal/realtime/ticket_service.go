package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// maxTickets bounds the in-memory ticket store so tickets minted but never
// validated cannot accumulate without bound.
const maxTickets = 1024

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
	return s.issue(ctx, userID, "")
}

// IssueForSession creates a ticket bound to one backend session.
func (s *TicketService) IssueForSession(ctx context.Context, userID, sessionID string) (Ticket, error) {
	return s.issue(ctx, userID, sessionID)
}

func (s *TicketService) issue(ctx context.Context, userID, sessionID string) (Ticket, error) {
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
	now := s.now().UTC()
	ticket := Ticket{Token: uuid.NewString(), UserID: userID, SessionID: sessionID, CircleIDs: circles, ExpiresAt: now.Add(TicketTTL)}
	s.mu.Lock()
	// Sweep expired tickets so ones minted but never validated do not linger,
	// then evict arbitrary entries if the store is still at its cap.
	for token, existing := range s.tokens {
		if existing.ExpiredAt(now) {
			delete(s.tokens, token)
		}
	}
	for len(s.tokens) >= maxTickets {
		for token := range s.tokens {
			delete(s.tokens, token)
			break
		}
	}
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
