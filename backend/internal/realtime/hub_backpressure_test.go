// Backpressure regression tests for finding
// realtime.hub.broadcast.global-mutex.no-write-deadline: broadcast used to
// hold the hub mutex across inline per-connection writes with no write
// deadline, so one stalled consumer blocked fan-out to all users and every
// upgrade handshake. These tests pin the pump-based fan-out: non-blocking
// enqueues under the hub mutex, a per-connection write pump with bounded
// deadlines, and slow-client eviction on queue overflow.
package realtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// pipeListener feeds an httptest.Server with synchronous net.Pipe connections
// so tests can stall hub writes deterministically: once a test stops reading
// from its pipe end, every further hub write to that connection blocks until
// the write deadline, with no OS socket buffers to absorb the frames.
type pipeListener struct {
	conns  chan net.Conn
	closed sync.Once
}

func newPipeListener() *pipeListener { return &pipeListener{conns: make(chan net.Conn, 8)} }

func (l *pipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.conns
	if !ok {
		return nil, errors.New("realtime test pipe listener closed")
	}
	return conn, nil
}

func (l *pipeListener) Close() error {
	l.closed.Do(func() { close(l.conns) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "realtime-test-pipe" }

// startPipeHub serves the hub behind a pipe listener. Tests must close every
// piped connection they dialed before the server can shut down, because the
// handlers only return once their connection dies.
func startPipeHub(t *testing.T, hub *Hub) (*httptest.Server, *pipeListener) {
	t.Helper()
	listener := newPipeListener()
	server := httptest.NewUnstartedServer(hub)
	server.Listener = listener
	server.Start()
	return server, listener
}

// stopPipeHub releases a pipe-served hub: closing the dialed ends unblocks any
// stalled hub-side writes and ends the handlers, letting server.Close finish.
func stopPipeHub(server *httptest.Server, conns ...*websocket.Conn) func() {
	return func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
		server.CloseClientConnections()
		server.Close()
	}
}

// dialHubPipe opens a hub connection over a synchronous net.Pipe. Once the
// test stops reading from the returned connection, every subsequent hub write
// to it blocks until the write deadline — a deterministic slow consumer.
func dialHubPipe(t *testing.T, listener *pipeListener, token string) *websocket.Conn {
	t.Helper()
	serverEnd, clientEnd := net.Pipe()
	select {
	case listener.conns <- serverEnd:
	case <-time.After(2 * time.Second):
		t.Fatal("test pipe listener did not accept the connection")
	}
	dialer := websocket.Dialer{
		NetDial:          func(string, string) (net.Conn, error) { return clientEnd, nil },
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := dialer.Dial("ws://halaqaty.test/ws?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func subscribePipe(t *testing.T, conn *websocket.Conn, topic string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	writeHub(t, conn, map[string]any{"action": realtimeActionSubscribe, "topic": topic})
	if got := readHub(t, conn); got["type"] != realtimeTypeSubscribed {
		t.Fatalf("subscribe response = %v", got)
	}
	_ = conn.SetReadDeadline(time.Time{})
}

// readClosedAfter waits out the write-deadline window without touching the
// connection — any earlier read would unblock a pending synchronous pipe
// write and defeat the stall — then probes the connection: a non-timeout read
// error means the hub closed it, a probe timeout names the real defect.
func readClosedAfter(conn *websocket.Conn, stall time.Duration, timeout error) error {
	time.Sleep(stall)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := conn.ReadMessage()
	var netErr net.Error
	if readErr != nil && errors.As(readErr, &netErr) && netErr.Timeout() {
		return timeout
	}
	return readErr
}

func TestHub_BroadcastDoesNotBlockBehindStalledConsumer(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server, listener := startPipeHub(t, hub)

	stalledTicket, _ := tickets.Issue(context.Background(), "user-1")
	fastTicket, _ := tickets.Issue(context.Background(), "user-2")
	stalled := dialHubPipe(t, listener, stalledTicket.Token)
	fast := dialHubPipe(t, listener, fastTicket.Token)
	defer stopPipeHub(server, stalled, fast)()

	sessionTopic := "session." + hubSessionID
	subscribePipe(t, stalled, sessionTopic)
	subscribePipe(t, fast, sessionTopic)

	// From here the stalled consumer never reads again: the next hub write to
	// it blocks on the synchronous pipe for the rest of the test.
	topic, _ := NewSessionTopic(hubSessionID)
	payload := map[string]any{"type": "session.hand_raised", "payload": map[string]any{"session_id": hubSessionID}}
	broadcastErr := make(chan error, 1)
	go func() { broadcastErr <- hub.Broadcast(topic, "stall-evt-1", payload) }()
	select {
	case err := <-broadcastErr:
		if err != nil {
			t.Fatalf("broadcast with stalled consumer: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast blocked behind a stalled consumer")
	}
	_ = fast.SetReadDeadline(time.Now().Add(2 * time.Second))
	if got := readHub(t, fast); got["type"] != "session.hand_raised" {
		t.Fatalf("healthy consumer event = %v", got)
	}
	// The hub mutex must also be free for follow-up fan-out.
	if err := hub.Broadcast(topic, "stall-evt-2", payload); err != nil {
		t.Fatalf("second broadcast: %v", err)
	}
	if got := readHub(t, fast); got["type"] != "session.hand_raised" {
		t.Fatalf("healthy consumer second event = %v", got)
	}
}

func TestHub_SlowConsumerClosedAfterWriteDeadline(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	hub.writeTimeout = 150 * time.Millisecond
	server, listener := startPipeHub(t, hub)

	stalledTicket, _ := tickets.Issue(context.Background(), "user-1")
	stalled := dialHubPipe(t, listener, stalledTicket.Token)
	defer stopPipeHub(server, stalled)()

	sessionTopic := "session." + hubSessionID
	subscribePipe(t, stalled, sessionTopic)

	topic, _ := NewSessionTopic(hubSessionID)
	closed := make(chan error, 1)
	go func() {
		if err := hub.Broadcast(topic, "deadline-evt", map[string]any{"type": "session.hand_raised"}); err != nil {
			closed <- fmt.Errorf("broadcast: %w", err)
			return
		}
		closed <- readClosedAfter(stalled, 400*time.Millisecond, errors.New("hub did not close the stalled connection before the probe read deadline"))
	}()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("stalled consumer unexpectedly received the stalled frame")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stalled consumer was not closed within the test window")
	}
}

func TestHub_QueueOverflowClosesSlowConsumer(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server, listener := startPipeHub(t, hub)

	stalledTicket, _ := tickets.Issue(context.Background(), "user-1")
	stalled := dialHubPipe(t, listener, stalledTicket.Token)
	defer stopPipeHub(server, stalled)()

	sessionTopic := "session." + hubSessionID
	subscribePipe(t, stalled, sessionTopic)

	topic, _ := NewSessionTopic(hubSessionID)
	payload := map[string]any{"type": "session.hand_raised"}
	closed := make(chan error, 1)
	go func() {
		for i := 0; i < realtimeSendBuffer+10; i++ {
			if err := hub.Broadcast(topic, fmt.Sprintf("overflow-%d", i), payload); err != nil {
				closed <- fmt.Errorf("broadcast %d: %w", i, err)
				return
			}
		}
		closed <- readClosedAfter(stalled, 0, errors.New("hub did not close the overflowing slow consumer"))
	}()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("slow consumer unexpectedly consumed every queued frame")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("overflowing slow consumer was not closed within the test window")
	}
}

func TestHub_UpgradeCompletesWhileStalledConsumerBlocksBroadcasts(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server, listener := startPipeHub(t, hub)

	stalledTicket, _ := tickets.Issue(context.Background(), "user-1")
	fastTicket, _ := tickets.Issue(context.Background(), "user-2")
	lateTicket, _ := tickets.Issue(context.Background(), "user-3")
	stalled := dialHubPipe(t, listener, stalledTicket.Token)
	fast := dialHubPipe(t, listener, fastTicket.Token)
	defer stopPipeHub(server, stalled, fast)()

	topic, _ := NewSessionTopic(hubSessionID)
	sessionTopic := "session." + hubSessionID
	subscribePipe(t, stalled, sessionTopic)
	subscribePipe(t, fast, sessionTopic)

	stormDone := make(chan struct{})
	go func() {
		defer close(stormDone)
		for i := 0; i < 50; i++ {
			_ = hub.Broadcast(topic, fmt.Sprintf("storm-%d", i), map[string]any{"type": "session.hand_raised"})
		}
	}()

	// A brand-new connection must still complete its upgrade handshake and
	// subscribe while fan-out targets the stalled consumer.
	late := dialHubPipe(t, listener, lateTicket.Token)
	otherTopic := "session.88888888-8888-8888-8888-888888888888"
	_ = late.SetReadDeadline(time.Now().Add(2 * time.Second))
	writeHub(t, late, map[string]any{"action": realtimeActionSubscribe, "topic": otherTopic})
	if got := readHub(t, late); got["type"] != realtimeTypeSubscribed {
		t.Fatalf("late subscribe response = %v, want subscribed", got)
	}
	<-stormDone
}

func TestHub_BroadcastPreservesPerConnectionFrameOrder(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()

	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	sessionTopic := "session." + hubSessionID
	writeHub(t, conn, map[string]any{"action": realtimeActionSubscribe, "topic": sessionTopic})
	if got := readHub(t, conn); got["type"] != realtimeTypeSubscribed {
		t.Fatalf("subscribe response = %v", got)
	}

	topic, _ := NewSessionTopic(hubSessionID)
	const frames = 10
	for i := 0; i < frames; i++ {
		payload := map[string]any{"type": "session.hand_raised", "payload": map[string]any{"sequence": i}}
		if err := hub.Broadcast(topic, fmt.Sprintf("order-%d", i), payload); err != nil {
			t.Fatalf("broadcast %d: %v", i, err)
		}
	}
	for i := 0; i < frames; i++ {
		got := readHub(t, conn)
		payload, _ := got["payload"].(map[string]any)
		if got["type"] != "session.hand_raised" || payload["sequence"] != float64(i) {
			t.Fatalf("frame %d = %v, want sequence %d in broadcast order", i, got, i)
		}
	}
}
