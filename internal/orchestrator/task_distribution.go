package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Zettaverse/agent-hub-core/internal/auth"
	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// Orchestrator distributes tasks across agents. It is concurrency-safe and
// safe for concurrent use by multiple callers.
type Orchestrator struct {
	store   store.Store
	mcp     mcp.Client
	decomp  Decomposer
	workers int
	timeout time.Duration
	now     func() time.Time
}

// Option configures an Orchestrator.
type Option func(*Orchestrator)

// WithWorkers sets the size of the bounded worker pool.
func WithWorkers(n int) Option {
	return func(o *Orchestrator) { o.workers = n }
}

// WithTimeout sets the per-subtask execution timeout.
func WithTimeout(d time.Duration) Option {
	return func(o *Orchestrator) { o.timeout = d }
}

// New returns an Orchestrator backed by the given Store and MCP client.
func New(s store.Store, c mcp.Client, opts ...Option) *Orchestrator {
	o := &Orchestrator{
		store:   s,
		mcp:     c,
		decomp:  Decomposer{},
		workers: 4,
		timeout: 30 * time.Second,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.workers < 1 {
		o.workers = 1
	}
	return o
}

// Distribute decomposes taskText into subtasks, assigns them to the best
// matching enabled agents, executes them concurrently, and aggregates the
// results in order. The returned Task reflects the final state (pending,
// running, success, or failed) and is persisted via the Store.
func Distribute(ctx context.Context, s store.Store, c mcp.Client, taskText string) (store.Task, error) {
	return New(s, c).Distribute(ctx, taskText)
}

func (o *Orchestrator) Distribute(ctx context.Context, taskText string) (store.Task, error) {
	tenantID := tenantFromCtx(ctx)
	agents, err := o.store.ListAgents(ctx, tenantID)
	if err != nil {
		return store.Task{}, err
	}

	specs := o.decomp.Decompose(taskText, agents)

	task := store.Task{
		ID:        newID(),
		TenantID:  tenantID,
		Input:     taskText,
		Status:    store.TaskStatusPending,
		Subtasks:  make([]store.Subtask, len(specs)),
		CreatedAt: o.now(),
		UpdatedAt: o.now(),
	}
	for i, spec := range specs {
		task.Subtasks[i] = store.Subtask{
			ID:          fmt.Sprintf("%s-%d", task.ID, i),
			AgentID:     spec.AgentID,
			Description: spec.Description,
			Status:      store.SubtaskStatusPending,
		}
	}

	task, err = o.store.CreateTask(ctx, task)
	if err != nil {
		return store.Task{}, err
	}

	task.Status = store.TaskStatusRunning
	task, _ = o.store.UpdateTask(ctx, task)

	results := make([]subtaskOutcome, len(specs))
	o.execute(ctx, task, specs, results)

	task = o.finalize(ctx, task, results)
	return task, nil
}

type subtaskOutcome struct {
	index  int
	result string
	err    error
}

func (o *Orchestrator) execute(ctx context.Context, task store.Task, specs []SubtaskSpec, results []subtaskOutcome) {
	sem := make(chan struct{}, o.workers)
	var wg sync.WaitGroup

	for i, spec := range specs {
		if ctx.Err() != nil {
			results[i] = subtaskOutcome{index: i, err: ctx.Err()}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, spec SubtaskSpec) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = o.runSubtask(ctx, task, i, spec)
		}(i, spec)
	}
	wg.Wait()
}

func (o *Orchestrator) runSubtask(ctx context.Context, task store.Task, idx int, spec SubtaskSpec) subtaskOutcome {
	out := subtaskOutcome{index: idx}

	markStatus := func(status store.SubtaskStatus) {
		if task.TenantID == "" {
			return
		}
		sub := task.Subtasks[idx]
		sub.Status = status
		if status == store.SubtaskStatusSuccess || status == store.SubtaskStatusFailed {
			b, _ := json.Marshal(out.result)
			sub.Result = b
		}
		task.Subtasks[idx] = sub
		task.UpdatedAt = o.now()
		updated, err := o.store.UpdateTask(ctx, task)
		if err == nil {
			task = updated
		}
	}

	if spec.AgentID == "" {
		out.err = errors.New("no matching agent for subtask")
		markStatus(store.SubtaskStatusFailed)
		return out
	}

	agent, err := o.store.GetAgent(ctx, task.TenantID, spec.AgentID)
	if err != nil {
		out.err = fmt.Errorf("load agent %s: %w", spec.AgentID, err)
		markStatus(store.SubtaskStatusFailed)
		return out
	}

	markStatus(store.SubtaskStatusRunning)

	subCtx := ctx
	var cancel context.CancelFunc
	if o.timeout > 0 {
		subCtx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}

	var outputs []string
	for _, skill := range spec.Skills {
		if subCtx.Err() != nil {
			break
		}
		server, err := o.store.GetMcpServer(ctx, task.TenantID, skill.ServerID)
		if err != nil {
			out.err = fmt.Errorf("load server %s: %w", skill.ServerID, err)
			break
		}
		transport, err := mcp.TransportFromServer(server.Transport)
		if err != nil {
			out.err = fmt.Errorf("parse transport for %s: %w", server.Name, err)
			break
		}
		res, err := o.mcp.CallTool(subCtx, transport, skill.Tool, map[string]any{
			"task":   spec.Description,
			"agent":  agent.Name,
			"prompt": agent.SystemPrompt,
		})
		if err != nil {
			out.err = fmt.Errorf("call tool %s: %w", skill.Tool, err)
			break
		}
		outputs = append(outputs, mcp.CallToolResult(res))
	}

	if out.err == nil && subCtx.Err() != nil {
		out.err = subCtx.Err()
	}

	if out.err != nil {
		out.result = joinNonEmpty(outputs)
		markStatus(store.SubtaskStatusFailed)
		return out
	}
	out.result = joinNonEmpty(outputs)
	markStatus(store.SubtaskStatusSuccess)
	return out
}

func (o *Orchestrator) finalize(ctx context.Context, task store.Task, results []subtaskOutcome) store.Task {
	sort.SliceStable(results, func(i, j int) bool { return results[i].index < results[j].index })

	var failed int
	aggregate := make([]string, 0, len(results))
	for i, r := range results {
		if r.err != nil {
			failed++
		}
		if r.result != "" {
			aggregate = append(aggregate, fmt.Sprintf("subtask %d: %s", i, r.result))
		}
	}

	task.Status = store.TaskStatusSuccess
	if failed > 0 || ctx.Err() != nil {
		task.Status = store.TaskStatusFailed
	}
	if ctx.Err() != nil {
		aggregate = append(aggregate, "context: "+ctx.Err().Error())
	}

	result := map[string]any{
		"subtasks_completed": len(results) - failed,
		"subtasks_failed":    failed,
		"results":            aggregate,
	}
	b, _ := json.Marshal(result)
	task.Result = b
	task.UpdatedAt = o.now()
	updated, err := o.store.UpdateTask(ctx, task)
	if err != nil {
		return task
	}
	return updated
}

func joinNonEmpty(parts []string) string {
	out := ""
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "\n"
		}
		out += p
	}
	return out
}

// tenantFromCtx extracts the tenant from a context carrying auth claims.
// It falls back to the default tenant for direct (non-HTTP) callers.
func tenantFromCtx(ctx context.Context) string {
	if id := auth.TenantIDFrom(ctx); id != "" {
		return id
	}
	return "default"
}

func newID() string {
	return uuid.NewString()
}
