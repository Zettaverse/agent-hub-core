package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestStdioHelper is a JSON-RPC responder process used by the stdio test. It
// reads newline-delimited JSON-RPC requests from stdin and replies on stdout.
func TestStdioHelper(t *testing.T) {
	if os.Getenv("GO_HELPER_PROCESS") != "1" {
		return
	}
	in := bufio.NewScanner(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(in.Bytes(), &req); err != nil {
			return
		}
		var result any
		switch req.Method {
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "description": "echo tool"}}}
		case "resources/list":
			result = map[string]any{"resources": []map[string]any{{"uri": "res://1", "name": "r1"}}}
		case "prompts/list":
			result = map[string]any{"prompts": []map[string]any{{"name": "p1"}}}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "hello from stdio"}}, "isError": false}
		case "resources/read":
			result = map[string]any{"contents": []map[string]any{{"type": "text", "text": "resource-body"}}}
		case "prompts/get":
			result = map[string]any{"description": "prompt", "messages": []map[string]any{}}
		default:
			result = map[string]any{}
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		b, _ := json.Marshal(resp)
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
	os.Exit(0)
}

func TestStdioJSONRPC(t *testing.T) {
	os.Setenv("GO_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_HELPER_PROCESS")

	client := NewClient()
	transport := Transport{Type: "stdio", Command: os.Args[0], Args: []string{"-test.run=TestStdioHelper"}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx, transport)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}

	res, err := client.CallTool(ctx, transport, "echo", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if CallToolResult(res) != "hello from stdio" {
		t.Fatalf("CallTool result = %q", CallToolResult(res))
	}

	resources, err := client.ListResources(ctx, transport)
	if err != nil || len(resources) != 1 {
		t.Fatalf("ListResources = %+v, err %v", resources, err)
	}
	prompts, err := client.ListPrompts(ctx, transport)
	if err != nil || len(prompts) != 1 {
		t.Fatalf("ListPrompts = %+v, err %v", prompts, err)
	}
}

func TestWebsocketJSONRPC(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if err := conn.ReadJSON(&req); err != nil {
				return
			}
			result := map[string]any{"content": []map[string]any{{"type": "text", "text": "ws-result"}}, "isError": false}
			_ = conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := client.CallTool(ctx, Transport{Type: "websocket", URL: wsURL}, "tool", nil)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if CallToolResult(res) != "ws-result" {
		t.Fatalf("result = %q", CallToolResult(res))
	}
}

func TestUnsupportedTransport(t *testing.T) {
	client := NewClient()
	_, err := client.ListTools(context.Background(), Transport{Type: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
}

func TestMockClient(t *testing.T) {
	mc := NewMockClient()
	res, err := mc.CallTool(context.Background(), Transport{}, "greet", map[string]any{"name": "x"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if CallToolResult(res) != `greet({"name":"x"})` {
		t.Fatalf("unexpected result %q", CallToolResult(res))
	}
	if len(mc.Calls()) != 1 {
		t.Fatalf("expected 1 recorded call, got %d", len(mc.Calls()))
	}
}
