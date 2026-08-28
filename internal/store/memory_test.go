package store

import (
	"context"
	"testing"
	"time"
)

func testAgent() Agent {
	return Agent{
		ID:       "agent-1",
		TenantID: "tenant-1",
		Name:     "Weather Agent",
		Enabled:  true,
		Skills:   []Skill{{Name: "weather", ServerID: "srv-1", Tool: "get_weather"}},
	}
}

func TestMemoryAgents(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	a := testAgent()

	if _, err := s.GetAgent(ctx, "tenant-1", "agent-1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := s.CreateAgent(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetAgent(ctx, "tenant-1", "agent-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Weather Agent" {
		t.Errorf("name = %q", got.Name)
	}
	// Cross-tenant isolation.
	if _, err := s.GetAgent(ctx, "other-tenant", "agent-1"); err != ErrNotFound {
		t.Fatalf("expected cross-tenant not found, got %v", err)
	}
	if err := s.DeleteAgent(ctx, "tenant-1", "agent-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetAgent(ctx, "tenant-1", "agent-1"); err != ErrNotFound {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestMemoryRunLifecycle(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	start := time.Now()
	run := Run{
		ID:        "run-1",
		FlowID:    "flow-1",
		TenantID:  "tenant-1",
		Status:    RunStatusPending,
		StartedAt: start,
		Logs:      []LogEntry{},
	}
	if _, err := s.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	run.Status = RunStatusSuccess
	if _, err := s.UpdateRun(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}
	got, err := s.GetRun(ctx, "tenant-1", "run-1")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != RunStatusSuccess {
		t.Errorf("status = %q, want success", got.Status)
	}
	runs, err := s.ListRuns(ctx, "tenant-1", "flow-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs = %d, err %v", len(runs), err)
	}
	runs, _ = s.ListRuns(ctx, "tenant-1", "other-flow")
	if len(runs) != 0 {
		t.Fatalf("expected no runs for other flow, got %d", len(runs))
	}
}

func TestMemoryUsersAndTenants(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	tenant := Tenant{ID: "tenant-1", Name: "default", CreatedAt: time.Now()}
	if _, err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	u := User{
		ID:           "user-1",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: "hash",
		Role:         RoleOwner,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if _, err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	got, err := s.GetUserByUsername(ctx, "tenant-1", "admin")
	if err != nil {
		t.Fatalf("get by username: %v", err)
	}
	if got.Role != RoleOwner {
		t.Errorf("role = %q", got.Role)
	}
}
