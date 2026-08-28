// Package flow implements the flow_json interpreter: graph validation,
// deterministic execution, node executors, a tiny expression language for
// conditions, and rollback of side effects.
package flow

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Node types supported by the flow engine.
const (
	NodeTypeTrigger   = "trigger"
	NodeTypeAgent     = "agent"
	NodeTypeMcpTool   = "mcp_tool"
	NodeTypeCondition = "condition"
	NodeTypeOutput    = "output"
)

// Trigger types.
const (
	TriggerManual   = "manual"
	TriggerSchedule = "schedule"
	TriggerEvent    = "event"
)

// Output kinds.
const (
	OutputDatabase = "database"
	OutputModbus   = "modbus"
	OutputFile     = "file"
	OutputHTTP     = "http"
)

// Node is a single vertex in a flow graph.
type Node struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`

	TriggerType string `json:"trigger_type,omitempty"`
	Schedule    string `json:"schedule,omitempty"`

	AgentID string `json:"agent_id,omitempty"`

	ServerID  string         `json:"server_id,omitempty"`
	Tool      string         `json:"tool,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`

	Expression string `json:"expression,omitempty"`

	Kind   string `json:"kind,omitempty"`
	Target string `json:"target,omitempty"`
	Value  any    `json:"value,omitempty"`
	Method string `json:"method,omitempty"`
}

// Edge is a directed edge between two nodes. SourceHandle/TargetHandle carry
// the handle names (e.g. a condition's "true"/"false").
type Edge struct {
	ID           string `json:"id,omitempty"`
	Source       string `json:"source"`
	SourceHandle string `json:"source_handle,omitempty"`
	Target       string `json:"target"`
	TargetHandle string `json:"target_handle,omitempty"`
}

// FlowDef is the top-level flow_json document.
type FlowDef struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Graph is a validated, immutable view of a flow.
type Graph struct {
	Nodes map[string]Node
	Edges []Edge
	adj   map[string][]Edge // source node id -> outgoing edges (in declaration order)
	order []string          // topological order of node ids
}

// ParseAndValidate parses flow_json and returns a validated Graph.
func ParseAndValidate(flowJSON json.RawMessage) (*Graph, error) {
	var def FlowDef
	if err := json.Unmarshal(flowJSON, &def); err != nil {
		return nil, fmt.Errorf("flow: malformed flow_json: %w", err)
	}
	g, err := buildGraph(def)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func buildGraph(def FlowDef) (*Graph, error) {
	if len(def.Nodes) == 0 {
		return nil, errors.New("flow: graph has no nodes")
	}
	g := &Graph{
		Nodes: make(map[string]Node, len(def.Nodes)),
		Edges: def.Edges,
		adj:   make(map[string][]Edge),
	}
	triggers := 0
	for _, n := range def.Nodes {
		if n.ID == "" {
			return nil, errors.New("flow: node with empty id")
		}
		if _, dup := g.Nodes[n.ID]; dup {
			return nil, fmt.Errorf("flow: duplicate node id %q", n.ID)
		}
		if err := validateNode(n); err != nil {
			return nil, err
		}
		if n.Type == NodeTypeTrigger {
			triggers++
		}
		g.Nodes[n.ID] = n
	}
	if triggers != 1 {
		return nil, fmt.Errorf("flow: expected exactly one trigger node, found %d", triggers)
	}

	// Build adjacency and validate edges.
	inDegree := make(map[string]int)
	for _, id := range nodeIDs(def.Nodes) {
		inDegree[id] = 0
	}
	for _, e := range def.Edges {
		if _, ok := g.Nodes[e.Source]; !ok {
			return nil, fmt.Errorf("flow: edge %q references unknown source node %q", e.ID, e.Source)
		}
		if _, ok := g.Nodes[e.Target]; !ok {
			return nil, fmt.Errorf("flow: edge %q references unknown target node %q", e.ID, e.Target)
		}
		g.adj[e.Source] = append(g.adj[e.Source], e)
		inDegree[e.Target]++
	}

	// Kahn's algorithm: topological sort + cycle detection.
	queue := make([]string, 0)
	for _, id := range nodeIDs(def.Nodes) {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	var order []string
	remaining := len(def.Nodes)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		remaining--
		for _, e := range g.adj[cur] {
			inDegree[e.Target]--
			if inDegree[e.Target] == 0 {
				queue = append(queue, e.Target)
			}
		}
	}
	if remaining != 0 {
		return nil, errors.New("flow: graph contains a cycle")
	}
	g.order = order
	return g, nil
}

// TopologicalOrder returns node ids in topological order.
func (g *Graph) TopologicalOrder() []string {
	out := make([]string, len(g.order))
	copy(out, g.order)
	return out
}

// OutgoingEdges returns the outgoing edges for a node, filtered by an optional
// source handle. An empty handle matches edges with no source handle.
func (g *Graph) OutgoingEdges(nodeID, handle string) []Edge {
	var out []Edge
	for _, e := range g.adj[nodeID] {
		if e.SourceHandle == handle {
			out = append(out, e)
		}
	}
	return out
}

// Trigger returns the single trigger node.
func (g *Graph) Trigger() (Node, error) {
	for _, n := range g.Nodes {
		if n.Type == NodeTypeTrigger {
			return n, nil
		}
	}
	return Node{}, errors.New("flow: no trigger node")
}

func validateNode(n Node) error {
	switch n.Type {
	case NodeTypeTrigger:
		if n.TriggerType != "" && n.TriggerType != TriggerManual && n.TriggerType != TriggerSchedule && n.TriggerType != TriggerEvent {
			return fmt.Errorf("flow: node %q has invalid trigger_type %q", n.ID, n.TriggerType)
		}
	case NodeTypeAgent:
		if n.AgentID == "" {
			return fmt.Errorf("flow: agent node %q missing agent_id", n.ID)
		}
	case NodeTypeMcpTool:
		if n.ServerID == "" {
			return fmt.Errorf("flow: mcp_tool node %q missing server_id", n.ID)
		}
		if n.Tool == "" {
			return fmt.Errorf("flow: mcp_tool node %q missing tool", n.ID)
		}
	case NodeTypeCondition:
		if n.Expression == "" {
			return fmt.Errorf("flow: condition node %q missing expression", n.ID)
		}
	case NodeTypeOutput:
		if !validOutputKind(n.Kind) {
			return fmt.Errorf("flow: output node %q has invalid kind %q", n.ID, n.Kind)
		}
		if n.Target == "" {
			return fmt.Errorf("flow: output node %q missing target", n.ID)
		}
	default:
		return fmt.Errorf("flow: node %q has invalid type %q", n.ID, n.Type)
	}
	return nil
}

func validOutputKind(kind string) bool {
	switch kind {
	case OutputDatabase, OutputModbus, OutputFile, OutputHTTP:
		return true
	}
	return false
}

func nodeIDs(nodes []Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}
