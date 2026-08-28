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
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

// cgoClient wraps the Rust libmcpengine C ABI. It is only compiled when the
// "mcpengine" build tag is supplied, and therefore is absent from the default
// Windows/CI build.
type cgoClient struct {
	mu sync.Mutex
}

// NewClient returns the cgo MCP client backed by libmcpengine.
func NewClient() Client {
	return &cgoClient{}
}

func (c *cgoClient) ListTools(ctx context.Context, s Transport) ([]Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	eng, err := c.connect(s)
	if err != nil {
		return nil, err
	}
	defer eng.free()
	out := eng.listTools()
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
	out := eng.listResources()
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
	out := eng.listPrompts()
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
	out := eng.callTool(name, string(argsJSON))
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
	out := eng.readResource(uri)
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
	out := eng.getPrompt(name, string(argsJSON))
	var res PromptResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Version returns the libmcpengine version string.
func Version() string {
	return C.GoString(C.mcp_engine_version())
}

func (c *cgoClient) connect(s Transport) (*engine, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	cstr := C.CString(string(raw))
	defer C.free(unsafe.Pointer(cstr))

	e := C.mcp_engine_new(cstr)
	if e == nil {
		return nil, fmt.Errorf("mcp: engine allocation failed")
	}
	if rc := C.mcp_engine_connect(e, cstr); rc != 0 {
		C.mcp_engine_free(e)
		return nil, fmt.Errorf("mcp: connect failed (rc=%d)", int(rc))
	}
	return &engine{ptr: e}, nil
}

// engine wraps an allocated mcp_engine_t handle.
type engine struct {
	ptr *C.mcp_engine_t
}

func (e *engine) free() {
	if e.ptr != nil {
		C.mcp_engine_free(e.ptr)
		e.ptr = nil
	}
}

func (e *engine) callTool(name, argsJSON string) string {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cArgs := C.CString(argsJSON)
	defer C.free(unsafe.Pointer(cArgs))
	out := C.mcp_engine_call_tool(e.ptr, cName, cArgs)
	return takeString(out)
}

func (e *engine) listTools() string {
	return takeString(C.mcp_engine_list_tools(e.ptr))
}

func (e *engine) listResources() string {
	return takeString(C.mcp_engine_list_resources(e.ptr))
}

func (e *engine) listPrompts() string {
	return takeString(C.mcp_engine_list_prompts(e.ptr))
}

func (e *engine) readResource(uri string) string {
	cURI := C.CString(uri)
	defer C.free(unsafe.Pointer(cURI))
	return takeString(C.mcp_engine_read_resource(e.ptr, cURI))
}

func (e *engine) getPrompt(name, argsJSON string) string {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cArgs := C.CString(argsJSON)
	defer C.free(unsafe.Pointer(cArgs))
	return takeString(C.mcp_engine_get_prompt(e.ptr, cName, cArgs))
}

// takeString copies a C string into Go-owned memory and frees the C buffer.
func takeString(cs *C.char) string {
	if cs == nil {
		return ""
	}
	s := C.GoString(cs)
	C.mcp_engine_string_free(cs)
	return s
}
