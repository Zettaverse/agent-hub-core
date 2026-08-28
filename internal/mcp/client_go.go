//go:build !mcpengine

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// goClient is the pure-Go MCP client: JSON-RPC 2.0 over stdio (spawn a child
// process, newline-delimited) or over a websocket.
type goClient struct {
	// dialTimeout bounds the time to establish a connection.
	dialTimeout time.Duration
}

// NewClient returns the default (pure-Go) MCP client.
func NewClient() Client {
	return &goClient{dialTimeout: 30 * time.Second}
}

func (c *goClient) ListTools(ctx context.Context, s Transport) ([]Tool, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := t.call(ctx, "tools/list", nil, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

func (c *goClient) ListResources(ctx context.Context, s Transport) ([]Resource, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	var out struct {
		Resources []Resource `json:"resources"`
	}
	if err := t.call(ctx, "resources/list", nil, &out); err != nil {
		return nil, err
	}
	return out.Resources, nil
}

func (c *goClient) ListPrompts(ctx context.Context, s Transport) ([]Prompt, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	var out struct {
		Prompts []Prompt `json:"prompts"`
	}
	if err := t.call(ctx, "prompts/list", nil, &out); err != nil {
		return nil, err
	}
	return out.Prompts, nil
}

func (c *goClient) CallTool(ctx context.Context, s Transport, name string, args map[string]any) (*ToolResult, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	var out ToolResult
	if err := t.call(ctx, "tools/call", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *goClient) ReadResource(ctx context.Context, s Transport, uri string) (*ResourceContent, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	params := map[string]any{"uri": uri}
	var out ResourceContent
	if err := t.call(ctx, "resources/read", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *goClient) GetPrompt(ctx context.Context, s Transport, name string, args map[string]any) (*PromptResult, error) {
	t, err := c.connect(ctx, s)
	if err != nil {
		return nil, err
	}
	defer t.close()
	params := map[string]any{"name": name}
	if args != nil {
		params["arguments"] = args
	}
	var out PromptResult
	if err := t.call(ctx, "prompts/get", params, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *goClient) connect(ctx context.Context, s Transport) (rpcTransport, error) {
	switch s.Type {
	case "stdio":
		cmd := exec.CommandContext(ctx, s.Command, s.Args...)
		return newStdioTransport(cmd)
	case "websocket":
		return newWSTransport(ctx, s.URL, c.dialTimeout)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedTransport, s.Type)
	}
}

// rpcTransport abstracts a synchronous JSON-RPC 2.0 request/response channel.
type rpcTransport interface {
	call(ctx context.Context, method string, params any, result any) error
	close() error
}

// jsonrpcRequest is the request envelope.
type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcErr     `json:"error,omitempty"`
}

type jsonrpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// stdioTransport speaks newline-delimited JSON-RPC 2.0 over a child process
// stdin/stdout. A mutex serializes request/response pairs, which is a valid
// strategy for a single duplex stdio stream.
type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	out    *bufio.Reader
	mu     sync.Mutex
	nextID int64
}

func newStdioTransport(cmd *exec.Cmd) (*stdioTransport, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start: %w", err)
	}
	return &stdioTransport{
		cmd:   cmd,
		stdin: stdin,
		out:   bufio.NewReader(stdout),
	}, nil
}

func (s *stdioTransport) call(ctx context.Context, method string, params any, result any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	req := jsonrpcRequest{JSONRPC: "2.0", ID: s.nextID, Method: method, Params: params}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := s.stdin.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("mcp: write: %w", err)
	}
	resp, err := s.readResponse(ctx)
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

func (s *stdioTransport) readResponse(ctx context.Context) (*jsonrpcResponse, error) {
	type result struct {
		resp *jsonrpcResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := s.out.ReadBytes('\n')
		if err != nil {
			ch <- result{err: err}
			return
		}
		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{resp: &resp}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("mcp: read: %w", r.err)
		}
		return r.resp, nil
	}
}

func (s *stdioTransport) close() error {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	return nil
}

// wsTransport speaks JSON-RPC 2.0 over a gorilla websocket.
type wsTransport struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	nextID int64
}

func newWSTransport(ctx context.Context, url string, timeout time.Duration) (*wsTransport, error) {
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: dial: %w", err)
	}
	return &wsTransport{conn: conn}, nil
}

func (w *wsTransport) call(ctx context.Context, method string, params any, result any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextID++
	req := jsonrpcRequest{JSONRPC: "2.0", ID: w.nextID, Method: method, Params: params}
	if err := w.conn.WriteJSON(req); err != nil {
		return fmt.Errorf("mcp: write: %w", err)
	}
	resp, err := w.readResponse(ctx)
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

func (w *wsTransport) readResponse(ctx context.Context) (*jsonrpcResponse, error) {
	type result struct {
		resp *jsonrpcResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		var resp jsonrpcResponse
		if err := w.conn.ReadJSON(&resp); err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{resp: &resp}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("mcp: read: %w", r.err)
		}
		return r.resp, nil
	}
}

func (w *wsTransport) close() error {
	if w.conn != nil {
		return w.conn.Close()
	}
	return nil
}

func decodeResponse(resp *jsonrpcResponse, result any) error {
	if resp.Error != nil {
		return fmt.Errorf("%w: [%d] %s", ErrRPCError, resp.Error.Code, resp.Error.Message)
	}
	if result == nil {
		return nil
	}
	if len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, result)
}
