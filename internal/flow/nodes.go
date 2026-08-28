package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// ErrPermissionDenied is returned when an output node's target is not allowed
// by the flow's permission boundary.
var ErrPermissionDenied = errors.New("flow: permission denied for output target")

// execContext carries per-run state through a single flow execution.
type execContext struct {
	engine  *Engine
	flow    store.Flow
	ctx     context.Context
	rb      *rollbackLog
	outputs map[string]any
	logs    []store.LogEntry
	seq     int
}

func (ec *execContext) log(level, message string) {
	ec.seq++
	ec.logs = append(ec.logs, store.LogEntry{
		Timestamp: ec.engine.now(),
		Level:     level,
		Message:   message,
		Source:    "flow",
	})
}

// executeNode runs a single node and returns its output value.
func (e *Engine) executeNode(ec *execContext, node Node, input any) (any, error) {
	switch node.Type {
	case NodeTypeTrigger:
		if node.Value != nil {
			return node.Value, nil
		}
		return map[string]any{
			"triggered": true,
			"type":      triggerTypeOr(node.TriggerType),
		}, nil

	case NodeTypeAgent:
		return e.runAgent(ec, node, input)

	case NodeTypeMcpTool:
		return e.runMcpTool(ec, node, input)

	case NodeTypeCondition:
		return evalExpression(node.Expression, map[string]any{"result": input})

	case NodeTypeOutput:
		return e.runOutput(ec, node, input)

	default:
		return nil, fmt.Errorf("flow: unknown node type %q", node.Type)
	}
}

func triggerTypeOr(t string) string {
	if t == "" {
		return TriggerManual
	}
	return t
}

func (e *Engine) runAgent(ec *execContext, node Node, input any) (any, error) {
	agent, err := e.store.GetAgent(ec.ctx, ec.flow.TenantID, node.AgentID)
	if err != nil {
		return nil, fmt.Errorf("flow: agent node %q: %w", node.ID, err)
	}
	ec.log("info", fmt.Sprintf("agent node %s -> agent %s", node.ID, agent.Name))

	var outputs []string
	for _, skill := range agent.Skills {
		server, err := e.store.GetMcpServer(ec.ctx, ec.flow.TenantID, skill.ServerID)
		if err != nil {
			return nil, fmt.Errorf("flow: agent node %q: load server %s: %w", node.ID, skill.ServerID, err)
		}
		transport, err := mcp.TransportFromServer(server.Transport)
		if err != nil {
			return nil, fmt.Errorf("flow: agent node %q: %w", node.ID, err)
		}
		res, err := e.mcp.CallTool(ec.ctx, transport, skill.Tool, map[string]any{
			"input":  input,
			"prompt": agent.SystemPrompt,
		})
		if err != nil {
			return nil, fmt.Errorf("flow: agent node %q: call %s: %w", node.ID, skill.Tool, err)
		}
		outputs = append(outputs, mcp.CallToolResult(res))
	}
	return map[string]any{
		"agent":  agent.Name,
		"output": strings.Join(outputs, "\n"),
	}, nil
}

func (e *Engine) runMcpTool(ec *execContext, node Node, input any) (any, error) {
	server, err := e.store.GetMcpServer(ec.ctx, ec.flow.TenantID, node.ServerID)
	if err != nil {
		return nil, fmt.Errorf("flow: mcp_tool node %q: %w", node.ID, err)
	}
	transport, err := mcp.TransportFromServer(server.Transport)
	if err != nil {
		return nil, fmt.Errorf("flow: mcp_tool node %q: %w", node.ID, err)
	}
	args := node.Arguments
	if args == nil {
		args = map[string]any{}
	}
	if _, ok := args["input"]; !ok {
		args["input"] = input
	}
	res, err := e.mcp.CallTool(ec.ctx, transport, node.Tool, args)
	if err != nil {
		return nil, fmt.Errorf("flow: mcp_tool node %q: call %s: %w", node.ID, node.Tool, err)
	}
	text := mcp.CallToolResult(res)
	ec.log("info", fmt.Sprintf("mcp_tool node %s -> %s/%s", node.ID, node.ServerID, node.Tool))

	// Register a rollback step for the tool side effect.
	if err := ec.rb.apply(ec.ctx, Effect{
		Kind:   "mcp_tool",
		Target: node.ServerID + "/" + node.Tool,
		Value:  text,
		Meta:   map[string]any{"server_id": node.ServerID, "tool": node.Tool, "args": args},
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"server_id": node.ServerID,
		"tool":      node.Tool,
		"output":    text,
	}, nil
}

func (e *Engine) runOutput(ec *execContext, node Node, input any) (any, error) {
	if err := checkOutputPermission(ec.flow, node); err != nil {
		return nil, err
	}
	value := resolveValue(node.Value, input)
	if err := ec.rb.apply(ec.ctx, Effect{
		Kind:   node.Kind,
		Target: node.Target,
		Value:  value,
		Meta:   map[string]any{"node": node.ID},
	}); err != nil {
		return nil, err
	}
	ec.log("info", fmt.Sprintf("output node %s -> %s:%s", node.ID, node.Kind, node.Target))
	return map[string]any{
		"kind":   node.Kind,
		"target": node.Target,
		"value":  value,
	}, nil
}

// checkOutputPermission enforces the flow permission boundary for
// side-effecting output kinds.
func checkOutputPermission(flow store.Flow, node Node) error {
	switch node.Kind {
	case OutputDatabase:
		if containsString(flow.Permissions.Databases, node.Target) {
			return nil
		}
	case OutputFile:
		if containsString(flow.Permissions.Files, node.Target) {
			return nil
		}
	case OutputModbus:
		if containsString(flow.Permissions.Resources, node.Target) {
			return nil
		}
	case OutputHTTP:
		// HTTP outputs are not bounded by the resource/file/database lists.
		return nil
	}
	return fmt.Errorf("%w: %s %q", ErrPermissionDenied, node.Kind, node.Target)
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func resolveValue(v any, input any) any {
	if s, ok := v.(string); ok {
		switch s {
		case "$input", "$result":
			return input
		}
	}
	return v
}

// emittedHandles returns the output handles a node emits given its output.
func emittedHandles(node Node, output any) []string {
	if node.Type == NodeTypeCondition {
		if truthy(output) {
			return []string{"true"}
		}
		return []string{"false"}
	}
	return []string{""}
}

// marshalResult converts a node output to JSON bytes for Run.Result.
func marshalResult(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
