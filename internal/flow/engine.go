package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// Engine executes flow_json definitions against a Store, MCP client, and
// SideEffectStore. It is safe for concurrent use: no mutable state is shared
// between executions.
type Engine struct {
	store       store.Store
	mcp         mcp.Client
	sideEffects SideEffectStore
	now         func() time.Time
}

// NewEngine constructs a flow Engine.
func NewEngine(s store.Store, c mcp.Client, se SideEffectStore) *Engine {
	if se == nil {
		se = NewMemorySideEffectStore()
	}
	return &Engine{store: s, mcp: c, sideEffects: se, now: time.Now}
}

// ExecutionResult is the outcome of a single flow execution.
type ExecutionResult struct {
	Status store.RunStatus
	Result json.RawMessage
	Logs   []store.LogEntry
	Err    error
}

// Validate parses and validates a flow_json document without executing it.
func (e *Engine) Validate(flowJSON json.RawMessage) error {
	_, err := ParseAndValidate(flowJSON)
	return err
}

// Execute runs a flow from its trigger, following edges deterministically and
// executing each node at most once. On any node error, side effects are rolled
// back in reverse order and the status is rolled_back.
func (e *Engine) Execute(ctx context.Context, fl store.Flow) ExecutionResult {
	res := ExecutionResult{Status: store.RunStatusFailed}

	g, err := ParseAndValidate(fl.FlowJSON)
	if err != nil {
		res.Err = err
		res.Logs = []store.LogEntry{{
			Timestamp: e.now(), Level: "error", Message: err.Error(), Source: "flow",
		}}
		return res
	}

	ec := &execContext{
		engine:  e,
		flow:    fl,
		ctx:     ctx,
		rb:      newRollbackLog(e.sideEffects),
		outputs: make(map[string]any),
	}

	trigger, err := g.Trigger()
	if err != nil {
		res.Err = err
		return res
	}

	triggerOut, err := e.executeNode(ec, trigger, nil)
	if err != nil {
		return e.rollbackAndFail(ec, err)
	}
	ec.outputs[trigger.ID] = triggerOut
	ec.log("info", fmt.Sprintf("node %s executed", trigger.ID))

	type queueItem struct{ nodeID string }
	queue := []queueItem{{nodeID: trigger.ID}}
	visited := map[string]bool{trigger.ID: true}
	var order []string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		node := g.Nodes[cur.nodeID]
		handles := emittedHandles(node, ec.outputs[cur.nodeID])
		for _, h := range handles {
			for _, edge := range g.OutgoingEdges(cur.nodeID, h) {
				if visited[edge.Target] {
					continue
				}
				visited[edge.Target] = true
				targetNode := g.Nodes[edge.Target]
				out, err := e.executeNode(ec, targetNode, ec.outputs[cur.nodeID])
				if err != nil {
					return e.rollbackAndFail(ec, err)
				}
				ec.outputs[edge.Target] = out
				ec.log("info", fmt.Sprintf("node %s executed", edge.Target))
				order = append(order, edge.Target)
				queue = append(queue, queueItem{nodeID: edge.Target})
			}
		}
	}

	resultMap := make(map[string]any, len(order)+1)
	resultMap[trigger.ID] = ec.outputs[trigger.ID]
	for _, id := range order {
		resultMap[id] = ec.outputs[id]
	}

	res.Status = store.RunStatusSuccess
	res.Result = marshalResult(resultMap)
	res.Logs = ec.logs
	return res
}

func (e *Engine) rollbackAndFail(ec *execContext, err error) ExecutionResult {
	errs := ec.rb.rollback(ec.ctx)
	ec.log("error", fmt.Sprintf("execution failed: %v", err))
	res := ExecutionResult{
		Status: store.RunStatusRolledBack,
		Result: marshalResult(map[string]any{
			"error":           err.Error(),
			"rollback_errors": len(errs),
		}),
		Logs: ec.logs,
		Err:  err,
	}
	return res
}
