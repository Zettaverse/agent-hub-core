package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func seedChatEnv(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	if _, err := e.store.CreateMcpServer(context.Background(), store.McpServer{
		ID:        "srv-1",
		TenantID:  defaultTenantID,
		Name:      "llama",
		Transport: json.RawMessage(`{"type":"stdio","command":"node"}`),
		Enabled:   true,
	}); err != nil {
		t.Fatalf("create mcp server: %v", err)
	}
	if _, err := e.store.CreateAgent(context.Background(), store.Agent{
		ID:           "agent-1",
		TenantID:     defaultTenantID,
		Name:         "chatbot",
		SystemPrompt: "be concise",
		Skills:       []store.Skill{{Name: "llama", ServerID: "srv-1", Tool: "chat"}},
		Enabled:      true,
	}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return e
}

func TestChatAgent(t *testing.T) {
	t.Run("single skill returns tool output", func(t *testing.T) {
		e := seedChatEnv(t)
		mc := mcp.NewMockClient()
		mc.Handler = func(_ mcp.Transport, name string, args map[string]any) (string, error) {
			if name != "chat" {
				t.Fatalf("tool name = %q, want chat", name)
			}
			if args["input"] != "hello" || args["prompt"] != "be concise" {
				t.Fatalf("args = %+v", args)
			}
			return "hello from llama", nil
		}
		e.srv.MCP = mc

		got, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-1", "hello")
		if err != nil {
			t.Fatalf("chatAgent: %v", err)
		}
		if got != "hello from llama" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("multiple skills join with newline", func(t *testing.T) {
		e := seedChatEnv(t)
		if _, err := e.store.CreateMcpServer(context.Background(), store.McpServer{
			ID:        "srv-2",
			TenantID:  defaultTenantID,
			Name:      "second",
			Transport: json.RawMessage(`{"type":"stdio","command":"node"}`),
			Enabled:   true,
		}); err != nil {
			t.Fatalf("create second server: %v", err)
		}
		if _, err := e.store.CreateAgent(context.Background(), store.Agent{
			ID:       "agent-2",
			TenantID: defaultTenantID,
			Name:     "multi",
			Skills: []store.Skill{
				{Name: "a", ServerID: "srv-1", Tool: "chat"},
				{Name: "b", ServerID: "srv-2", Tool: "summarize"},
			},
			Enabled: true,
		}); err != nil {
			t.Fatalf("create multi agent: %v", err)
		}

		mc := mcp.NewMockClient()
		mc.Handler = func(_ mcp.Transport, name string, _ map[string]any) (string, error) {
			return "out-" + name, nil
		}
		e.srv.MCP = mc

		got, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-2", "x")
		if err != nil {
			t.Fatalf("chatAgent: %v", err)
		}
		if got != "out-chat\nout-summarize" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("agent with no skills errors", func(t *testing.T) {
		e := seedChatEnv(t)
		if _, err := e.store.CreateAgent(context.Background(), store.Agent{
			ID:       "agent-noskill",
			TenantID: defaultTenantID,
			Name:     "empty",
			Enabled:  true,
		}); err != nil {
			t.Fatalf("create no-skill agent: %v", err)
		}
		_, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-noskill", "x")
		if err == nil || !strings.Contains(err.Error(), "agent has no skills") {
			t.Fatalf("err = %v, want agent has no skills", err)
		}
	})

	t.Run("missing agent errors", func(t *testing.T) {
		e := seedChatEnv(t)
		_, err := e.srv.chatAgent(context.Background(), defaultTenantID, "missing", "x")
		if err == nil || !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want store.ErrNotFound", err)
		}
	})

	t.Run("missing mcp server errors", func(t *testing.T) {
		e := seedChatEnv(t)
		if _, err := e.store.CreateAgent(context.Background(), store.Agent{
			ID:       "agent-badserver",
			TenantID: defaultTenantID,
			Name:     "bad",
			Skills:   []store.Skill{{Name: "a", ServerID: "nope", Tool: "chat"}},
			Enabled:  true,
		}); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		_, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-badserver", "x")
		if err == nil || !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("err = %v, want store.ErrNotFound", err)
		}
	})

	t.Run("invalid transport errors", func(t *testing.T) {
		e := seedChatEnv(t)
		if _, err := e.store.CreateMcpServer(context.Background(), store.McpServer{
			ID:        "srv-bad",
			TenantID:  defaultTenantID,
			Name:      "bad transport",
			Transport: json.RawMessage(`not-json`),
			Enabled:   true,
		}); err != nil {
			t.Fatalf("create server: %v", err)
		}
		if _, err := e.store.CreateAgent(context.Background(), store.Agent{
			ID:       "agent-badtransport",
			TenantID: defaultTenantID,
			Name:     "bad",
			Skills:   []store.Skill{{Name: "a", ServerID: "srv-bad", Tool: "chat"}},
			Enabled:  true,
		}); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		_, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-badtransport", "x")
		if err == nil || !strings.Contains(err.Error(), "parse transport") {
			t.Fatalf("err = %v, want parse transport error", err)
		}
	})

	t.Run("tool call error propagates", func(t *testing.T) {
		e := seedChatEnv(t)
		mc := mcp.NewMockClient()
		mc.Handler = func(_ mcp.Transport, _ string, _ map[string]any) (string, error) {
			return "", errors.New("llm down")
		}
		e.srv.MCP = mc
		_, err := e.srv.chatAgent(context.Background(), defaultTenantID, "agent-1", "x")
		if err == nil || !strings.Contains(err.Error(), "llm down") {
			t.Fatalf("err = %v, want llm down", err)
		}
	})
}
