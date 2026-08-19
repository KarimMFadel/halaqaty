package realtime

import (
	"context"
	"testing"
	"time"
)

type ticketReader struct{ ids []string }

func (r ticketReader) ListCircleIDs(context.Context, string) ([]string, error) { return r.ids, nil }

func TestTicketServiceIssueAndValidate(t *testing.T) {
	s := NewTicketService(ticketReader{ids: []string{"00000000-0000-0000-0000-000000000001"}})
	ticket, err := s.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Token == "" || len(ticket.CircleIDs) != 1 || ticket.ExpiresAt.Before(time.Now()) {
		t.Fatalf("invalid ticket: %+v", ticket)
	}
	validated, err := s.Validate(ticket.Token, "user-1")
	if err != nil || validated.Token != ticket.Token {
		t.Fatalf("validate: %v, %+v", err, validated)
	}
	if _, err := s.Validate(ticket.Token, "user-2"); err == nil {
		t.Fatal("mismatched user ticket accepted")
	}
}

func TestTicketServiceExpiresTickets(t *testing.T) {
	s := NewTicketService(ticketReader{})
	now := time.Now()
	s.now = func() time.Time { return now }
	ticket, err := s.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(TicketTTL)
	if _, err := s.Validate(ticket.Token, "user-1"); err == nil {
		t.Fatal("expired ticket accepted")
	}
}
