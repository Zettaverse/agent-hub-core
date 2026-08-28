package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
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

type wsPayload struct {
	Type    string `json:"type"`
	Tenant  string `json:"tenant"`
	Payload struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		AgentID string `json:"agent_id"`
	} `json:"payload"`
	Time string `json:"time"`
}

func TestWSChatRoundTrip(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	// Seed an MCP server and an enabled agent whose single skill points at
	// that server's tool, directly through the store (the mock MCP client
	// never touches the transport, but TransportFromServer must parse it).
	if _, err := e.store.CreateMcpServer(context.Background(), store.McpServer{
		ID:        "srv-chat",
		TenantID:  defaultTenantID,
		Name:      "llama",
		Transport: json.RawMessage(`{"type":"stdio","command":"node"}`),
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	if _, err := e.store.CreateAgent(context.Background(), store.Agent{
		ID:           "agent-chat",
		TenantID:     defaultTenantID,
		Name:         "chatbot",
		SystemPrompt: "you are a helpful assistant",
		Skills:       []store.Skill{{Name: "llama", ServerID: "srv-chat", Tool: "chat"}},
		Enabled:      true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	mc := mcp.NewMockClient()
	mc.Handler = func(_ mcp.Transport, _ string, _ map[string]any) (string, error) {
		return "hello from llama", nil
	}
	e.srv.MCP = mc

	wsURL := "ws" + strings.TrimPrefix(e.ts.URL, "http") + "/api/v1/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, msg, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read greeting: %v", err)
	} else if !strings.Contains(string(msg), "connected") {
		t.Fatalf("greeting = %s", msg)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"chat","agent_id":"agent-chat","content":"hi"}`)); err != nil {
		t.Fatalf("write chat: %v", err)
	}

	// First broadcast is the echoed user turn, second is the assistant reply.
	_, userRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read user turn: %v", err)
	}
	_, asstRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read assistant turn: %v", err)
	}

	var userMsg, asstMsg wsPayload
	if err := json.Unmarshal(userRaw, &userMsg); err != nil {
		t.Fatalf("unmarshal user turn: %v", err)
	}
	if err := json.Unmarshal(asstRaw, &asstMsg); err != nil {
		t.Fatalf("unmarshal assistant turn: %v", err)
	}

	if userMsg.Type != "agent_message" || userMsg.Payload.Role != "user" {
		t.Fatalf("user turn = %+v", userMsg)
	}
	if userMsg.Payload.Content != "hi" || userMsg.Payload.AgentID != "agent-chat" {
		t.Fatalf("user turn payload = %+v", userMsg.Payload)
	}
	if asstMsg.Type != "agent_message" || asstMsg.Payload.Role != "assistant" {
		t.Fatalf("assistant turn = %+v", asstMsg)
	}
	if !strings.Contains(asstMsg.Payload.Content, "hello from llama") {
		t.Fatalf("assistant content = %q", asstMsg.Payload.Content)
	}
	if asstMsg.Payload.AgentID != "agent-chat" {
		t.Fatalf("assistant agent_id = %q", asstMsg.Payload.AgentID)
	}
}

func TestWSChatPicksFirstEnabledAgent(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	if _, err := e.store.CreateMcpServer(context.Background(), store.McpServer{
		ID:        "srv-auto",
		TenantID:  defaultTenantID,
		Name:      "llama",
		Transport: json.RawMessage(`{"type":"stdio","command":"node"}`),
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	if _, err := e.store.CreateAgent(context.Background(), store.Agent{
		ID:       "agent-disabled",
		TenantID: defaultTenantID,
		Name:     "off",
		Skills:   []store.Skill{{Name: "llama", ServerID: "srv-auto", Tool: "chat"}},
		Enabled:  false,
	}); err != nil {
		t.Fatalf("create disabled agent: %v", err)
	}
	if _, err := e.store.CreateAgent(context.Background(), store.Agent{
		ID:           "agent-enabled",
		TenantID:     defaultTenantID,
		Name:         "on",
		SystemPrompt: "prompt",
		Skills:       []store.Skill{{Name: "llama", ServerID: "srv-auto", Tool: "chat"}},
		Enabled:      true,
	}); err != nil {
		t.Fatalf("create enabled agent: %v", err)
	}

	mc := mcp.NewMockClient()
	mc.Handler = func(_ mcp.Transport, _ string, _ map[string]any) (string, error) {
		return "auto reply", nil
	}
	e.srv.MCP = mc

	wsURL := "ws" + strings.TrimPrefix(e.ts.URL, "http") + "/api/v1/ws?token=" + tok
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, msg, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read greeting: %v", err)
	} else if !strings.Contains(string(msg), "connected") {
		t.Fatalf("greeting = %s", msg)
	}

	// No agent_id: the server must select the first enabled agent.
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"chat","content":"yo"}`)); err != nil {
		t.Fatalf("write chat: %v", err)
	}

	_, userRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read user turn: %v", err)
	}
	_, asstRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read assistant turn: %v", err)
	}

	var userMsg, asstMsg wsPayload
	if err := json.Unmarshal(userRaw, &userMsg); err != nil {
		t.Fatalf("unmarshal user turn: %v", err)
	}
	if err := json.Unmarshal(asstRaw, &asstMsg); err != nil {
		t.Fatalf("unmarshal assistant turn: %v", err)
	}

	if userMsg.Payload.AgentID != "agent-enabled" {
		t.Fatalf("user turn agent_id = %q, want agent-enabled", userMsg.Payload.AgentID)
	}
	if asstMsg.Payload.Role != "assistant" || asstMsg.Payload.AgentID != "agent-enabled" {
		t.Fatalf("assistant turn = %+v", asstMsg)
	}
	if !strings.Contains(asstMsg.Payload.Content, "auto reply") {
		t.Fatalf("assistant content = %q", asstMsg.Payload.Content)
	}
}
