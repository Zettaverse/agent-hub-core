package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// chatMessage is the inbound client -> server chat frame.
type chatMessage struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
}

// chatAgentTimeout bounds a single agent turn so a slow or hung local LLM
// cannot wedge the connection forever.
const chatAgentTimeout = 120 * time.Second

// handleWS upgrades the request to a WebSocket and streams agent activity
// logs. The JWT is supplied as a query parameter (?token=...). Inbound chat
// messages are routed to an agent (whose MCP skills call a local LLM) and the
// reply is broadcast back to every client in the tenant.
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
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"agent_message","payload":{"role":"system","content":"connected"}}`))

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
	// which closes the send channel and terminates the write pump. The loop may
	// block inside chatAgent while the LLM runs; the write pump goroutine keeps
	// delivering broadcasts in the meantime.
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg chatMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Type != "chat" || msg.Content == "" {
			continue
		}

		agentID := msg.AgentID
		if agentID == "" {
			agents, err := s.Store.ListAgents(r.Context(), claims.TenantID)
			if err != nil {
				s.broadcastChat(claims.TenantID, "system", "error listing agents: "+err.Error(), "")
				continue
			}
			for _, a := range agents {
				if a.Enabled {
					agentID = a.ID
					break
				}
			}
			if agentID == "" {
				s.broadcastChat(claims.TenantID, "system", "no enabled agent available", "")
				continue
			}
		}

		// Echo the user's turn so the UI can render the conversation.
		s.broadcastChat(claims.TenantID, "user", msg.Content, agentID)

		ctx, cancel := context.WithTimeout(r.Context(), chatAgentTimeout)
		reply, err := s.chatAgent(ctx, claims.TenantID, agentID, msg.Content)
		cancel()
		if err != nil {
			s.broadcastChat(claims.TenantID, "system", "error: "+err.Error(), agentID)
			continue
		}
		s.broadcastChat(claims.TenantID, "assistant", reply, agentID)
	}
}

// broadcastChat emits a chat payload on the tenant room. The agent_id field is
// omitted for system broadcasts that are not tied to a specific agent (e.g. "no
// enabled agent available").
func (s *Server) broadcastChat(tenantID, role, content, agentID string) {
	payload := map[string]any{
		"role":    role,
		"content": content,
	}
	if agentID != "" {
		payload["agent_id"] = agentID
	}
	s.Hub.Broadcast(tenantID, "agent_message", payload)
}
