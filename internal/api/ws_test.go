package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSHandler(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	wsURL := "ws" + strings.TrimPrefix(e.ts.URL, "http") + "/api/v1/ws?token=" + tok
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial: %v (http %d)", err, resp.StatusCode)
		}
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.Contains(string(msg), "connected") {
		t.Fatalf("greeting = %s", msg)
	}
}

func TestWSHandlerRejectsBadToken(t *testing.T) {
	e := newTestEnv(t)
	wsURL := "ws" + strings.TrimPrefix(e.ts.URL, "http") + "/api/v1/ws?token=garbage"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial error for bad token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}
}
