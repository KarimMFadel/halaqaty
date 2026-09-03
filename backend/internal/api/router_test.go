package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRouter_OnlyRealtimeWebSocketBypassesTimeout(t *testing.T) {
	router := NewRouter(MiddlewareSet{Timeout: time.Millisecond})
	router.mux.Handle("GET /slow", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	router.mux.Handle(routeRealtimeWebSocket, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
	}))

	server := httptest.NewServer(router.Handler())
	defer server.Close()

	t.Run("ordinary endpoint with upgrade header still times out", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/slow", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Upgrade", "websocket")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
		}
	})

	t.Run("realtime websocket upgrades", func(t *testing.T) {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/api/v1/ws", nil)
		if err != nil {
			t.Fatalf("dial websocket: %v", err)
		}
		defer func() { _ = conn.Close() }()
	})
}
