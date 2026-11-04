package flow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zettaverse/agent-hub-core/internal/store"
)

// TestDAGJoinWaitsForDependencies checks ordering across paths of unequal length.
func TestDAGJoinWaitsForDependencies(t *testing.T) {
	e, _ := testEngine(t)
	f := mkFlow(`{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"a","type":"output","kind":"database","target":"db1"},
			{"id":"b","type":"output","kind":"database","target":"db2"},
			{"id":"c","type":"output","kind":"database","target":"db_true"},
			{"id":"join","type":"output","kind":"database","target":"db_false"}
		],
		"edges":[
			{"source":"t","target":"a"}, {"source":"t","target":"b"},
			{"source":"a","target":"join"}, {"source":"b","target":"c"},
			{"source":"c","target":"join"}
		]
	}`, allowAll())
	result := e.Execute(context.Background(), f)
	if result.Status != store.RunStatusSuccess {
		t.Fatalf("flow failed: status=%s error=%v", result.Status, result.Err)
	}
	var order []string
	for _, entry := range result.Logs {
		if strings.HasPrefix(entry.Message, "node ") && strings.HasSuffix(entry.Message, " executed") {
			order = append(order, strings.TrimSuffix(strings.TrimPrefix(entry.Message, "node "), " executed"))
		}
	}
	positions := make(map[string]int, len(order))
	for i, id := range order {
		positions[id] = i
	}
	c, cOK := positions["c"]
	join, joinOK := positions["join"]
	if !cOK || !joinOK || c >= join {
		t.Fatalf("join must follow c; execution order: %v", order)
	}
}

func TestDAGJoinWithInactiveConditionalBranch(t *testing.T) {
	for _, tc := range []struct{ name, value, active, skipped string }{
		{"true", "20", "yes", "no"},
		{"false", "3", "no", "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := testEngine(t)
			f := mkFlow(`{"nodes":[
				{"id":"t","type":"trigger","value":{"value":`+tc.value+`}},
				{"id":"condition","type":"condition","expression":"result.value > 10"},
				{"id":"yes","type":"output","kind":"database","target":"db_true"},
				{"id":"no","type":"output","kind":"database","target":"db_false"},
				{"id":"join","type":"output","kind":"database","target":"db1"}],
				"edges":[{"source":"t","target":"condition"},
				{"source":"condition","source_handle":"true","target":"yes"},
				{"source":"condition","source_handle":"false","target":"no"},
				{"source":"yes","target":"join"},{"source":"no","target":"join"}]}`, allowAll())
			r := e.Execute(context.Background(), f)
			if r.Status != store.RunStatusSuccess {
				t.Fatalf("status=%s err=%v", r.Status, r.Err)
			}
			var outputs map[string]any
			if err := json.Unmarshal(r.Result, &outputs); err != nil {
				t.Fatal(err)
			}
			if _, ok := outputs[tc.active]; !ok {
				t.Fatalf("active branch missing: %s", tc.active)
			}
			if _, ok := outputs[tc.skipped]; ok {
				t.Fatalf("inactive branch executed: %s", tc.skipped)
			}
			if _, ok := outputs["join"]; !ok {
				t.Fatal("join did not execute")
			}
		})
	}
}

func TestDAGJoinUsesFirstActiveIncomingEdge(t *testing.T) {
	e, effects := testEngine(t)
	f := mkFlow(`{"nodes":[
		{"id":"t","type":"trigger"},
		{"id":"a","type":"output","kind":"database","target":"db1","value":"first"},
		{"id":"b","type":"output","kind":"database","target":"db2","value":"second"},
		{"id":"join","type":"output","kind":"database","target":"db_true","value":"$input"}],
		"edges":[{"source":"t","target":"a"},{"source":"t","target":"b"},
		{"source":"b","target":"join"},{"source":"a","target":"join"}]}`, allowAll())
	r := e.Execute(context.Background(), f)
	if r.Status != store.RunStatusSuccess {
		t.Fatalf("status=%s err=%v", r.Status, r.Err)
	}
	value, ok := effects.Value("database", "db_true")
	if !ok {
		t.Fatal("missing join output")
	}
	input, ok := value.(map[string]any)
	if !ok || input["target"] != "db2" {
		t.Fatalf("join input = %#v, want output from b", value)
	}
}
