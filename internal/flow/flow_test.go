package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zettaverse/agent-hub-core/internal/mcp"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

func testEngine(t *testing.T) (*Engine, *MemorySideEffectStore) {
	t.Helper()
	s := store.NewMemoryStore()
	ctx := context.Background()
	_, _ = s.CreateTenant(ctx, store.Tenant{ID: "t1", Name: "t1", CreatedAt: time.Now()})
	_, _ = s.CreateMcpServer(ctx, store.McpServer{
		ID: "srv1", TenantID: "t1", Name: "srv",
		Transport: json.RawMessage(`{"type":"stdio","command":"echo"}`), Enabled: true,
	})
	_, _ = s.CreateAgent(ctx, store.Agent{
		ID: "agent1", TenantID: "t1", Name: "Agent", Enabled: true,
		Skills: []store.Skill{{Name: "weather", ServerID: "srv1", Tool: "get_weather"}},
	})
	se := NewMemorySideEffectStore()
	e := NewEngine(s, mcp.NewMockClient(), se)
	return e, se
}

func mkFlow(flowJSON string, perms store.FlowPermissionSet) store.Flow {
	return store.Flow{
		ID: "f1", TenantID: "t1", Name: "test",
		FlowJSON: json.RawMessage(flowJSON), Permissions: perms, Enabled: true,
	}
}

func allowAll() store.FlowPermissionSet {
	return store.FlowPermissionSet{
		Resources: []string{"modbus1"}, Files: []string{"/tmp/a.txt"}, Databases: []string{"db1", "db2", "db_true", "db_false"},
	}
}

func TestGraphValidation(t *testing.T) {
	cases := []struct {
		name    string
		flow    string
		wantErr string
	}{
		{"malformed json", `{not json`, "malformed"},
		{"no nodes", `{"nodes":[],"edges":[]}`, "no nodes"},
		{"no trigger", `{"nodes":[{"id":"o","type":"output","kind":"database","target":"db1"}],"edges":[]}`, "trigger"},
		{"two triggers", `{"nodes":[{"id":"t1","type":"trigger"},{"id":"t2","type":"trigger"}],"edges":[]}`, "trigger"},
		{"invalid type", `{"nodes":[{"id":"t","type":"trigger"},{"id":"x","type":"bogus"}],"edges":[{"source":"t","target":"x"}]}`, "invalid type"},
		{"agent missing id", `{"nodes":[{"id":"t","type":"trigger"},{"id":"a","type":"agent"}],"edges":[{"source":"t","target":"a"}]}`, "agent_id"},
		{"dangling edge", `{"nodes":[{"id":"t","type":"trigger"},{"id":"o","type":"output","kind":"database","target":"db1"}],"edges":[{"source":"t","target":"missing"}]}`, "unknown target"},
		{"condition missing expression", `{"nodes":[{"id":"t","type":"trigger"},{"id":"c","type":"condition"}],"edges":[{"source":"t","target":"c"}]}`, "expression"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAndValidate(json.RawMessage(tc.flow))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCycleRejection(t *testing.T) {
	// t -> a -> b -> a (cycle)
	flow := `{
		"nodes": [
			{"id":"t","type":"trigger"},
			{"id":"a","type":"output","kind":"database","target":"db1"},
			{"id":"b","type":"output","kind":"database","target":"db2"}
		],
		"edges": [
			{"source":"t","target":"a"},
			{"source":"a","target":"b"},
			{"source":"b","target":"a"}
		]
	}`
	_, err := ParseAndValidate(json.RawMessage(flow))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestSelfLoopRejection(t *testing.T) {
	flow := `{
		"nodes": [{"id":"t","type":"trigger"}],
		"edges": [{"source":"t","target":"t"}]
	}`
	_, err := ParseAndValidate(json.RawMessage(flow))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error for self-loop, got %v", err)
	}
}

func TestSuccessfulRun(t *testing.T) {
	e, se := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"o","type":"output","kind":"database","target":"db1","value":"hello"}
		],
		"edges":[{"source":"t","target":"o"}]
	}`, allowAll())

	res := e.Execute(context.Background(), fl)
	if res.Status != store.RunStatusSuccess {
		t.Fatalf("status = %q, err = %v", res.Status, res.Err)
	}
	if v, ok := se.Value("database", "db1"); !ok || v != "hello" {
		t.Fatalf("db1 = %v, want hello", v)
	}
}

func TestNodeFailureRollback(t *testing.T) {
	e, se := testEngine(t)
	ctx := context.Background()
	// Pre-populate db1 so we can assert rollback restores the prior value.
	_, _ = se.Apply(ctx, Effect{Kind: "database", Target: "db1", Value: "original"})

	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"o1","type":"output","kind":"database","target":"db1","value":"new"},
			{"id":"o2","type":"output","kind":"database","target":"db2","value":"v2"},
			{"id":"bad","type":"agent","agent_id":"nonexistent"}
		],
		"edges":[
			{"source":"t","target":"o1"},
			{"source":"o1","target":"o2"},
			{"source":"o2","target":"bad"}
		]
	}`, allowAll())

	res := e.Execute(ctx, fl)
	if res.Status != store.RunStatusRolledBack {
		t.Fatalf("status = %q, want rolled_back", res.Status)
	}
	// db1 must be restored to its prior value.
	if v, ok := se.Value("database", "db1"); !ok || v != "original" {
		t.Fatalf("db1 = %v, want original (rollback did not restore)", v)
	}
	// db2 must be rolled back (deleted).
	if _, ok := se.Value("database", "db2"); ok {
		t.Fatal("db2 should have been rolled back")
	}
	// Undo order must be reverse of apply order: db2 then db1.
	log := se.UndoLog()
	if len(log) != 2 {
		t.Fatalf("expected 2 undo records, got %d", len(log))
	}
	if log[0].Effect.Target != "db2" || log[1].Effect.Target != "db1" {
		t.Fatalf("undo order = [%s, %s], want [db2, db1]", log[0].Effect.Target, log[1].Effect.Target)
	}
}

func TestPermissionDenied(t *testing.T) {
	e, _ := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"o","type":"output","kind":"database","target":"secret_db","value":"x"}
		],
		"edges":[{"source":"t","target":"o"}]
	}`, store.FlowPermissionSet{Databases: []string{"allowed_db"}})

	res := e.Execute(context.Background(), fl)
	if !errors.Is(res.Err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", res.Err)
	}
	if res.Status != store.RunStatusRolledBack {
		t.Fatalf("status = %q, want rolled_back", res.Status)
	}
}

func TestConditionBranching(t *testing.T) {
	e, se := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger","value":{"value":20}},
			{"id":"c","type":"condition","expression":"result.value > 10"},
			{"id":"ot","type":"output","kind":"database","target":"db_true","value":"yes"},
			{"id":"of","type":"output","kind":"database","target":"db_false","value":"no"}
		],
		"edges":[
			{"source":"t","target":"c"},
			{"source":"c","source_handle":"true","target":"ot"},
			{"source":"c","source_handle":"false","target":"of"}
		]
	}`, allowAll())

	res := e.Execute(context.Background(), fl)
	if res.Status != store.RunStatusSuccess {
		t.Fatalf("status = %q, err = %v", res.Status, res.Err)
	}
	if v, ok := se.Value("database", "db_true"); !ok || v != "yes" {
		t.Fatalf("db_true = %v, want yes", v)
	}
	if _, ok := se.Value("database", "db_false"); ok {
		t.Fatal("false branch should not have executed")
	}
}

func TestConditionFalseBranch(t *testing.T) {
	e, se := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger","value":{"value":3}},
			{"id":"c","type":"condition","expression":"result.value > 10"},
			{"id":"ot","type":"output","kind":"database","target":"db_true","value":"yes"},
			{"id":"of","type":"output","kind":"database","target":"db_false","value":"no"}
		],
		"edges":[
			{"source":"t","target":"c"},
			{"source":"c","source_handle":"true","target":"ot"},
			{"source":"c","source_handle":"false","target":"of"}
		]
	}`, allowAll())

	res := e.Execute(context.Background(), fl)
	if res.Status != store.RunStatusSuccess {
		t.Fatalf("status = %q, err = %v", res.Status, res.Err)
	}
	if _, ok := se.Value("database", "db_true"); ok {
		t.Fatal("true branch should not have executed")
	}
	if v, ok := se.Value("database", "db_false"); !ok || v != "no" {
		t.Fatalf("db_false = %v, want no", v)
	}
}

func TestDeterministicOrdering(t *testing.T) {
	e, _ := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"a","type":"output","kind":"database","target":"db1","value":"a"},
			{"id":"b","type":"output","kind":"database","target":"db2","value":"b"},
			{"id":"c","type":"output","kind":"database","target":"db_true","value":"c"}
		],
		"edges":[
			{"source":"t","target":"a"},
			{"source":"t","target":"b"},
			{"source":"t","target":"c"}
		]
	}`, allowAll())

	for i := 0; i < 5; i++ {
		res := e.Execute(context.Background(), fl)
		if res.Status != store.RunStatusSuccess {
			t.Fatalf("run %d status = %q", i, res.Status)
		}
		var msgs []string
		for _, l := range res.Logs {
			if strings.HasPrefix(l.Message, "node ") {
				msgs = append(msgs, strings.TrimSuffix(strings.TrimPrefix(l.Message, "node "), " executed"))
			}
		}
		want := []string{"t", "a", "b", "c"}
		if len(msgs) != len(want) {
			t.Fatalf("run %d execution order = %v, want %v", i, msgs, want)
		}
		for j := range want {
			if msgs[j] != want[j] {
				t.Fatalf("run %d execution order = %v, want %v", i, msgs, want)
			}
		}
	}
}

func TestMalformedFlowJSONFails(t *testing.T) {
	e, _ := testEngine(t)
	fl := mkFlow(`{"nodes":[{"id":"t","type":"trigger"}],"edges":[]}`, allowAll())
	fl.FlowJSON = json.RawMessage(`{"nodes":`)
	res := e.Execute(context.Background(), fl)
	if res.Status != store.RunStatusFailed {
		t.Fatalf("status = %q, want failed", res.Status)
	}
}

func TestConcurrentRunsSameFlow(t *testing.T) {
	e, _ := testEngine(t)
	fl := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"o","type":"output","kind":"database","target":"db1","value":"v"}
		],
		"edges":[{"source":"t","target":"o"}]
	}`, allowAll())

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := e.Execute(context.Background(), fl)
			if res.Status != store.RunStatusSuccess {
				errs <- errors.New("run not successful: " + string(res.Result))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
