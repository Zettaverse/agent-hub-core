package api

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// handleWS upgrades the request to a WebSocket and streams agent activity
// logs. The JWT is supplied as a query parameter (?token=...).
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing token")
		return
	}
	claims, err := s.Auth.Verify(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	send, deregister := s.Hub.Register(claims.TenantID)
	defer deregister()

	// Greeting.
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"agent_message","payload":{"message":"connected"}}`))

	// Write pump: forwards hub broadcasts to the client. It terminates when
	// the send channel is closed (by deregister) or a write fails.
	go func() {
		for msg := range send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	// Read pump: this is the handler's main loop. When the client disconnects
	// the loop returns, triggering the deferred conn.Close() and deregister(),
	// which closes the send channel and terminates the write pump.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
