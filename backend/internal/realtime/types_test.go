package realtime

import (
	"testing"
	"time"
)

const (
	testCircleID  = "00000000-0000-0000-0000-000000000001"
	testCircleID2 = "00000000-0000-0000-0000-000000000002"
	testSessionID = "00000000-0000-0000-0000-000000000003"
)

func TestNewCircleTopic(t *testing.T) {
	topic, err := NewCircleTopic(testCircleID)

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if topic.Kind() != TopicCircle {
		t.Fatalf("kind: got %q, want %q", topic.Kind(), TopicCircle)
	}
	if topic.ID() != testCircleID {
		t.Fatalf("id: got %q, want %q", topic.ID(), testCircleID)
	}
	if got, want := topic.String(), "circle."+testCircleID; got != want {
		t.Fatalf("wire form: got %q, want %q", got, want)
	}
}

func TestNewSessionTopic(t *testing.T) {
	topic, err := NewSessionTopic(testSessionID)

	if err != nil {
		t.Fatalf("error: got %v, want nil", err)
	}
	if topic.Kind() != TopicSession {
		t.Fatalf("kind: got %q, want %q", topic.Kind(), TopicSession)
	}
	if got, want := topic.String(), "session."+testSessionID; got != want {
		t.Fatalf("wire form: got %q, want %q", got, want)
	}
}

func TestTopicConstructors_RejectInvalidUUID(t *testing.T) {
	for name, build := range map[string]func(string) (Topic, error){
		"circle":  NewCircleTopic,
		"session": NewSessionTopic,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := build("not-a-uuid")

			if err == nil {
				t.Fatal("error: got nil, want validation failure")
			}
		})
	}
}

func TestParseTopic(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Topic
		wantErr bool
	}{
		{name: "circle topic", raw: "circle." + testCircleID, want: mustTopic(t, TopicCircle, testCircleID)},
		{name: "session topic", raw: "session." + testSessionID, want: mustTopic(t, TopicSession, testSessionID)},
		{name: "missing separator", raw: testCircleID, wantErr: true},
		{name: "unknown kind", raw: "chat." + testCircleID, wantErr: true},
		{name: "empty kind", raw: "." + testCircleID, wantErr: true},
		{name: "invalid uuid", raw: "circle.not-a-uuid", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTopic(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("error: got nil, want failure for %q", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("error: got %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("topic: got %q, want %q", got, tt.want)
			}
			// Round-trip: the parsed topic must render back to the wire form.
			if got.String() != tt.raw {
				t.Fatalf("round-trip: got %q, want %q", got.String(), tt.raw)
			}
		})
	}
}

func TestTicket_ExpiredAt(t *testing.T) {
	now := time.Now()
	ticket := Ticket{ExpiresAt: now.Add(TicketTTL)}

	if ticket.ExpiredAt(now) {
		t.Fatal("fresh ticket reported expired")
	}
	if !ticket.ExpiredAt(ticket.ExpiresAt) {
		t.Fatal("ticket at expiry time reported valid")
	}
	if !ticket.ExpiredAt(ticket.ExpiresAt.Add(time.Second)) {
		t.Fatal("past-expiry ticket reported valid")
	}
}

func TestTicket_Covers(t *testing.T) {
	ticket := Ticket{CircleIDs: []string{testCircleID}}
	circle, err := NewCircleTopic(testCircleID)
	if err != nil {
		t.Fatalf("circle topic: %v", err)
	}
	otherCircle, err := NewCircleTopic(testCircleID2)
	if err != nil {
		t.Fatalf("other circle topic: %v", err)
	}
	// A session topic sharing a listed UUID must still not be covered.
	session, err := NewSessionTopic(testCircleID)
	if err != nil {
		t.Fatalf("session topic: %v", err)
	}

	if !ticket.Covers(circle) {
		t.Fatal("eligible circle topic not covered")
	}
	if ticket.Covers(otherCircle) {
		t.Fatal("unlisted circle topic covered")
	}
	if ticket.Covers(session) {
		t.Fatal("session topic covered by a generic ticket; session topics require an authorized join")
	}
}

func TestConnectionStateValues(t *testing.T) {
	tests := map[ConnectionState]string{
		ConnectionConnecting:   "connecting",
		ConnectionConnected:    "connected",
		ConnectionReconnecting: "reconnecting",
		ConnectionDisconnected: "disconnected",
		ConnectionClosed:       "closed",
	}
	for state, want := range tests {
		if string(state) != want {
			t.Fatalf("connection state: got %q, want %q", string(state), want)
		}
	}
}

// mustTopic builds a topic with a known-valid kind for table expectations.
func mustTopic(t *testing.T, kind TopicKind, id string) Topic {
	t.Helper()
	topic, err := newTopic(kind, id)
	if err != nil {
		t.Fatalf("build %s topic: %v", kind, err)
	}
	return topic
}
