// Package mcp provides a Model Context Protocol client with two selectable
// backends: a pure-Go JSON-RPC 2.0 implementation (default, build tag
// !mcpengine) and a cgo binding to the Rust libmcpengine library (build tag
// mcpengine).
package mcp

import (
	"context"
	"encoding/json"
	"errors"
)

// Transport describes how to reach an MCP server.
type Transport struct {
	Type    string   `json:"type"` // "stdio" or "websocket"
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	URL     string   `json:"url,omitempty"`
}

// TransportFromServer parses the json.RawMessage transport config stored on a
// store.McpServer into a Transport.
func TransportFromServer(raw json.RawMessage) (Transport, error) {
	var t Transport
	if err := json.Unmarshal(raw, &t); err != nil {
		return Transport{}, err
	}
	return t, nil
}

// Tool describes a callable tool exposed by an MCP server.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// Resource describes a readable resource exposed by an MCP server.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

// Prompt describes a prompt template exposed by an MCP server.
type Prompt struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ToolResult is the outcome of a tool call.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock is a single content item returned by a tool/resource/prompt.
type ContentBlock struct {
	Type     string `json:"type"` // "text" or "image"
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// ResourceContent is the result of reading a resource.
type ResourceContent struct {
	Contents []ContentBlock `json:"contents"`
}

// PromptResult is the result of getting a prompt.
type PromptResult struct {
	Description string          `json:"description"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage is a single message in a prompt result.
type PromptMessage struct {
	Role    string       `json:"role"`
	Content ContentBlock `json:"content"`
}

// Client is the MCP client interface. All methods take a Transport describing
// how to reach the server.
type Client interface {
	ListTools(ctx context.Context, server Transport) ([]Tool, error)
	ListResources(ctx context.Context, server Transport) ([]Resource, error)
	ListPrompts(ctx context.Context, server Transport) ([]Prompt, error)
	CallTool(ctx context.Context, server Transport, name string, args map[string]any) (*ToolResult, error)
	ReadResource(ctx context.Context, server Transport, uri string) (*ResourceContent, error)
	GetPrompt(ctx context.Context, server Transport, name string, args map[string]any) (*PromptResult, error)
}

// Errors returned by client implementations.
var (
	ErrUnsupportedTransport = errors.New("mcp: unsupported transport type")
	ErrRPCError             = errors.New("mcp: json-rpc error")
)

// CallToolResult is a convenience helper returning the textual content of a
// tool call joined with newlines.
func CallToolResult(r *ToolResult) string {
	out := ""
	for i, b := range r.Content {
		if i > 0 {
			out += "\n"
		}
		out += b.Text
	}
	return out
}
