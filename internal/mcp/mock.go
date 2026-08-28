package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// MockClient is an in-memory Client implementation for tests. Tool results are
// produced by calling a handler function, defaulting to echoing the tool name
// and arguments as JSON.
type MockClient struct {
	mu      sync.Mutex
	Tools   []Tool
	Handler func(server Transport, name string, args map[string]any) (string, error)
	calls   []CallRecord
}

// CallRecord captures a single CallTool invocation for assertions.
type CallRecord struct {
	Server Transport
	Name   string
	Args   map[string]any
}

// NewMockClient returns a MockClient with sensible defaults.
func NewMockClient() *MockClient {
	return &MockClient{}
}

// Calls returns a copy of the recorded CallTool invocations.
func (m *MockClient) Calls() []CallRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CallRecord, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *MockClient) ListTools(ctx context.Context, s Transport) ([]Tool, error) {
	return m.Tools, nil
}

func (m *MockClient) ListResources(ctx context.Context, s Transport) ([]Resource, error) {
	return nil, nil
}

func (m *MockClient) ListPrompts(ctx context.Context, s Transport) ([]Prompt, error) {
	return nil, nil
}

func (m *MockClient) CallTool(ctx context.Context, s Transport, name string, args map[string]any) (*ToolResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, CallRecord{Server: s, Name: name, Args: args})
	m.mu.Unlock()

	if m.Handler != nil {
		out, err := m.Handler(s, name, args)
		if err != nil {
			return nil, err
		}
		return &ToolResult{Content: []ContentBlock{{Type: "text", Text: out}}}, nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	return &ToolResult{Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("%s(%s)", name, string(b))}}}, nil
}

func (m *MockClient) ReadResource(ctx context.Context, s Transport, uri string) (*ResourceContent, error) {
	return &ResourceContent{Contents: []ContentBlock{{Type: "text", Text: uri}}}, nil
}

func (m *MockClient) GetPrompt(ctx context.Context, s Transport, name string, args map[string]any) (*PromptResult, error) {
	return &PromptResult{Description: name}, nil
}
