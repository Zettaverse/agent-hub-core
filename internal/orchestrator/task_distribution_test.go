package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func setupStore(t *testing.T) *store.Memory {
	t.Helper()
	s := store.NewMemoryStore()
	ctx := context.Background()
	_, _ = s.CreateTenant(ctx, store.Tenant{ID: "default", Name: "default", CreatedAt: time.Now()})
	_, _ = s.CreateMcpServer(ctx, store.McpServer{
		ID:        "srv-1",
		TenantID:  "default",
		Name:      "srv1",
		Transport: json.RawMessage(`{"type":"stdio","command":"echo"}`),
		Enabled:   true,
		Status:    "connected",
	})
	_, _ = s.CreateAgent(ctx, store.Agent{
		ID:       "agent-1",
		TenantID: "default",
		Name:     "Weather",
		Enabled:  true,
		Skills:   []store.Skill{{Name: "weather", ServerID: "srv-1", Tool: "get_weather"}},
	})
	_, _ = s.CreateAgent(ctx, store.Agent{
		ID:       "agent-2",
		TenantID: "default",
		Name:     "Translate",
		Enabled:  true,
		Skills:   []store.Skill{{Name: "translate", ServerID: "srv-1", Tool: "translate_text"}},
	})
	return s
}

type fakeClient struct {
	mcp.Client
	err        error
	blockOnCtx bool
	mu         sync.Mutex
	calls      []string
}

func (f *fakeClient) CallTool(ctx context.Context, s mcp.Transport, name string, args map[string]any) (*mcp.ToolResult, error) {
	if f.blockOnCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &mcp.ToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: "result-of-" + name}}}, nil
}

func TestDecompose(t *testing.T) {
	agents := []store.Agent{
		{ID: "a1", Name: "Weather", Enabled: true, Skills: []store.Skill{{Name: "weather", Tool: "get_weather"}}},
		{ID: "a2", Name: "Translate", Enabled: true, Skills: []store.Skill{{Name: "translate", Tool: "translate_text"}}},
		{ID: "a3", Name: "Disabled", Enabled: false, Skills: []store.Skill{{Name: "weather", Tool: "x"}}},
	}
	d := Decomposer{}

	specs := d.Decompose("get the weather and translate it", agents)
	if len(specs) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(specs))
	}
	if specs[0].AgentID != "a1" {
		t.Errorf("first subtask agent = %q, want a1", specs[0].AgentID)
	}
	if specs[1].AgentID != "a2" {
		t.Errorf("second subtask agent = %q, want a2", specs[1].AgentID)
	}

	// No match -> generic subtask.
	specs = d.Decompose("do something completely unrelated", agents)
	if len(specs) != 1 {
		t.Fatalf("expected 1 generic subtask, got %d", len(specs))
	}
	if specs[0].AgentID != "" {
		t.Errorf("generic subtask should have empty AgentID, got %q", specs[0].AgentID)
	}
}

func TestDistributeBasic(t *testing.T) {
	s := setupStore(t)
	client := &fakeClient{}
	o := New(s, client, WithWorkers(2))

	ctx := context.Background()
	task, err := o.Distribute(ctx, "get the weather and translate it")
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if task.Status != store.TaskStatusSuccess {
		t.Fatalf("status = %q, want success (result=%s)", task.Status, task.Result)
	}
	if len(task.Subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(task.Subtasks))
	}
	for _, sub := range task.Subtasks {
		if sub.Status != store.SubtaskStatusSuccess {
			t.Errorf("subtask %s status = %q, want success", sub.ID, sub.Status)
		}
	}
	// Ensure tools were actually invoked.
	client.mu.Lock()
	calls := len(client.calls)
	client.mu.Unlock()
	if calls != 2 {
		t.Errorf("expected 2 tool calls, got %d", calls)
	}
}

func TestDistributeNoAgents(t *testing.T) {
	s := store.NewMemoryStore()
	ctx := context.Background()
	_, _ = s.CreateTenant(ctx, store.Tenant{ID: "default", Name: "default", CreatedAt: time.Now()})
	o := New(s, &fakeClient{})

	task, err := o.Distribute(ctx, "something with no matching skills")
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if task.Status != store.TaskStatusFailed {
		t.Fatalf("status = %q, want failed", task.Status)
	}
}

func TestDistributeAgentErrorContinues(t *testing.T) {
	s := setupStore(t)
	client := &fakeClient{err: errors.New("tool exploded")}
	o := New(s, client, WithWorkers(2))

	task, err := o.Distribute(context.Background(), "get the weather and translate it")
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if task.Status != store.TaskStatusFailed {
		t.Fatalf("status = %q, want failed", task.Status)
	}
	for _, sub := range task.Subtasks {
		if sub.Status != store.SubtaskStatusFailed {
			t.Errorf("subtask %s status = %q, want failed", sub.ID, sub.Status)
		}
	}
}

func TestDistributeTimeout(t *testing.T) {
	s := setupStore(t)
	client := &fakeClient{blockOnCtx: true}
	o := New(s, client, WithWorkers(1), WithTimeout(30*time.Millisecond))

	start := time.Now()
	task, err := o.Distribute(context.Background(), "get the weather")
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if task.Status != store.TaskStatusFailed {
		t.Fatalf("status = %q, want failed (timeout)", task.Status)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("distribute did not respect timeout, took %v", elapsed)
	}
}

func TestDistributeConcurrent(t *testing.T) {
	s := setupStore(t)
	client := &fakeClient{}
	o := New(s, client, WithWorkers(4))

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := o.Distribute(context.Background(), "get the weather and translate it")
			if err != nil {
				errs <- err
				return
			}
			if task.Status != store.TaskStatusSuccess {
				errs <- errors.New("task not successful")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent distribute error: %v", err)
	}
}

func TestDistributeContextCancellation(t *testing.T) {
	s := setupStore(t)
	client := &fakeClient{blockOnCtx: true}
	o := New(s, client, WithWorkers(1))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task, err := o.Distribute(ctx, "get the weather")
	if err != nil {
		t.Fatalf("distribute: %v", err)
	}
	if task.Status != store.TaskStatusFailed {
		t.Fatalf("status = %q, want failed", task.Status)
	}
}
