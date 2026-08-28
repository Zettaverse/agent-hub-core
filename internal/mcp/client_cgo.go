//go:build mcpengine

package mcp

/*
#cgo LDFLAGS: -lmcpengine
#cgo linux LDFLAGS: -L${SRCDIR}/../../third_party/mcp-engine/target/release
#include <stdlib.h>
#include "../../third_party/mcp-engine/include/mcpengine.h"
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

// errBufSize is the fixed size of the C error buffers handed to libmcpengine.
const errBufSize = 1024

// cgoClient wraps the Rust libmcpengine C ABI. It is compiled only when the
// "mcpengine" build tag is supplied, and therefore is absent from the default
// Windows/CI build.
type cgoClient struct {
	mu sync.Mutex
}

// NewClient returns the cgo MCP client backed by libmcpengine.
func NewClient() Client {
	return &cgoClient{}
}

// Version returns the libmcpengine version string.
func Version() string {
	return C.GoString(C.mcp_engine_version())
}

func (c *cgoClient) ListTools(ctx context.Context, s Transport) ([]Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	out, err := eng.listTools()
	if err != nil {
		return nil, err
	}
	var res struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

func (c *cgoClient) ListResources(ctx context.Context, s Transport) ([]Resource, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	out, err := eng.listResources()
	if err != nil {
		return nil, err
	}
	var res struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return res.Resources, nil
}

func (c *cgoClient) ListPrompts(ctx context.Context, s Transport) ([]Prompt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	out, err := eng.listPrompts()
	if err != nil {
		return nil, err
	}
	var res struct {
		Prompts []Prompt `json:"prompts"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return res.Prompts, nil
}

func (c *cgoClient) CallTool(ctx context.Context, s Transport, name string, args map[string]any) (*ToolResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	out, err := eng.callTool(name, string(argsJSON))
	if err != nil {
		return nil, err
	}
	var res ToolResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *cgoClient) ReadResource(ctx context.Context, s Transport, uri string) (*ResourceContent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	out, err := eng.readResource(uri)
	if err != nil {
		return nil, err
	}
	var res ResourceContent
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *cgoClient) GetPrompt(ctx context.Context, s Transport, name string, args map[string]any) (*PromptResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	out, err := eng.getPrompt(name, string(argsJSON))
	if err != nil {
		return nil, err
	}
	var res PromptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// serverIDFor derives a stable per-transport identifier used as the map key in
// libmcpengine's connection pool.
func serverIDFor(s Transport) string {
	if s.URL != "" {
		return "ws:" + s.URL
	}
	return "stdio:" + s.Command
}

// connect allocates a libmcpengine engine and connects it to the transport.
func (c *cgoClient) connect(s Transport) (*engine, error) {
	configJSON := `{"log_level":"info","max_message_bytes":1048576,"sanitize":true,"connect_timeout_ms":10000}`
	cCfg := C.CString(configJSON)
	defer C.free(unsafe.Pointer(cCfg))

	errBuf := make([]byte, errBufSize)
	e := C.mcp_engine_new(cCfg, (*C.char)(unsafe.Pointer(&errBuf[0])), errBufSize)
	if e == nil {
		return nil, fmt.Errorf("mcp: engine allocation failed: %s", errString(errBuf))
	}
	eng := &engine{ptr: e, serverID: serverIDFor(s)}

	raw, err := json.Marshal(s)
	if err != nil {
		eng.free()
		return nil, err
	}

	cID := C.CString(eng.serverID)
	defer C.free(unsafe.Pointer(cID))
	cTransport := C.CString(string(raw))
	defer C.free(unsafe.Pointer(cTransport))

	if rc := C.mcp_engine_connect(e, cID, cTransport, (*C.char)(unsafe.Pointer(&errBuf[0])), errBufSize); rc != 0 {
		eng.free()
		return nil, fmt.Errorf("mcp: connect failed (rc=%d): %s", int(rc), errString(errBuf))
	}
	return eng, nil
}

// engine wraps an allocated mcp_engine_t handle.
type engine struct {
	ptr      *C.mcp_engine_t
	serverID string
}

func (e *engine) free() {
	if e.ptr != nil {
		C.mcp_engine_free(e.ptr)
		e.ptr = nil
	}
}

// callString invokes a C function returning a malloc'd JSON string, allocating
// a stack error buffer for diagnostics. It returns the decoded string or an
// error sourced from the error buffer when the C function returns NULL.
func (e *engine) callString(fn func(*C.char) *C.char) (string, error) {
	errBuf := make([]byte, errBufSize)
	out := fn((*C.char)(unsafe.Pointer(&errBuf[0])))
	if out == nil {
		return "", fmt.Errorf("mcp: %s", errString(errBuf))
	}
	defer C.mcp_engine_string_free(out)
	return C.GoString(out), nil
}

func (e *engine) listTools() (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_list_tools(e.ptr, cID, buf, errBufSize)
	})
}

func (e *engine) listResources() (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_list_resources(e.ptr, cID, buf, errBufSize)
	})
}

func (e *engine) listPrompts() (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_list_prompts(e.ptr, cID, buf, errBufSize)
	})
}

func (e *engine) callTool(name, argsJSON string) (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cArgs := C.CString(argsJSON)
	defer C.free(unsafe.Pointer(cArgs))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_call_tool(e.ptr, cID, cName, cArgs, buf, errBufSize)
	})
}

func (e *engine) readResource(uri string) (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	cURI := C.CString(uri)
	defer C.free(unsafe.Pointer(cURI))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_read_resource(e.ptr, cID, cURI, buf, errBufSize)
	})
}

func (e *engine) getPrompt(name, argsJSON string) (string, error) {
	cID := C.CString(e.serverID)
	defer C.free(unsafe.Pointer(cID))
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cArgs := C.CString(argsJSON)
	defer C.free(unsafe.Pointer(cArgs))
	return e.callString(func(buf *C.char) *C.char {
		return C.mcp_engine_get_prompt(e.ptr, cID, cName, cArgs, buf, errBufSize)
	})
}

// errString converts a NUL-terminated C error buffer into a trimmed Go string.
func errString(buf []byte) string {
	if i := bytes.IndexByte(buf, 0); i >= 0 {
		return string(buf[:i])
	}
	return string(buf)
}
