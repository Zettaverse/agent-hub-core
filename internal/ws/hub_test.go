package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubBroadcastToTenant(t *testing.T) {
	hub := NewHub()
	var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		send, deregister := hub.Register("tenant-a")
		defer deregister()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for msg := range send {
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Give the server a moment to register the client.
	deadline := time.Now().Add(2 * time.Second)
	for hub.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", hub.ClientCount())
	}

	hub.Broadcast("tenant-a", "agent_message", map[string]any{"message": "hello"})

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read broadcast: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Type != "agent_message" {
		t.Fatalf("type = %q, want agent_message", msg.Type)
	}
}

func TestHubTenantIsolation(t *testing.T) {
	hub := NewHub()
	received := make(chan []byte, 1)
	// Simulate a client by registering directly and reading its channel.
	sendA, deregA := hub.Register("tenant-a")
	defer deregA()
	sendB, deregB := hub.Register("tenant-b")
	defer deregB()

	go func() {
		for msg := range sendA {
			received <- msg
		}
	}()

	hub.Broadcast("tenant-b", "run_update", map[string]any{"x": 1})

	select {
	case msg := <-received:
		t.Fatalf("tenant-a received message meant for tenant-b: %s", msg)
	case <-time.After(200 * time.Millisecond):
		// expected: no delivery to tenant-a
	}

	hub.Broadcast("tenant-a", "task_update", map[string]any{"y": 2})
	select {
	case msg := <-received:
		if !strings.Contains(string(msg), "task_update") {
			t.Fatalf("unexpected message: %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("tenant-a did not receive its broadcast")
	}

	// Drain tenant-b to avoid blocking on its buffered channel is not needed.
	_ = sendB
}
