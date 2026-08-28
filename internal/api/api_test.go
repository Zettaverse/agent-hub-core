package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zettaverse/agent-hub-core/internal/config"
	"github.com/Zettaverse/agent-hub-core/internal/store"
)

type testEnv struct {
	srv   *Server
	ts    *httptest.Server
	store store.Store
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st := store.NewMemoryStore()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		JWTSecret:         "test-secret",
		JWTExpiry:         time.Hour,
		SeedOwnerUsername: "admin",
		SeedOwnerPassword: "admin123",
		TaskWorkerPool:    2,
		TaskTimeout:       5 * time.Second,
	}
	s := NewServer(cfg, st, logger)
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	return &testEnv{srv: s, ts: ts, store: st}
}

func (e *testEnv) token(t *testing.T, role string) string {
	t.Helper()
	tok, err := e.srv.Auth.Issue("user", "user-"+role, defaultTenantID, role)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func (e *testEnv) do(t *testing.T, method, path, body, token string) (*http.Response, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, string(data)
}

func TestLoginSeedsOwnerAndMe(t *testing.T) {
	e := newTestEnv(t)

	resp, body := e.do(t, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"admin123"}`, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", resp.StatusCode, body)
	}
	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &loginResp); err != nil || loginResp.Token == "" {
		t.Fatalf("login response %q", body)
	}

	resp, body = e.do(t, http.MethodGet, "/api/v1/me", "", loginResp.Token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", resp.StatusCode, body)
	}
	var me store.User
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		t.Fatalf("unmarshal me: %v", err)
	}
	if me.Username != "admin" || me.Role != store.RoleOwner {
		t.Fatalf("me = %+v", me)
	}
}

func TestLoginBadCredentials(t *testing.T) {
	e := newTestEnv(t)
	resp, _ := e.do(t, http.MethodPost, "/api/v1/auth/login", `{"username":"admin","password":"wrong"}`, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestRBACEnforcement(t *testing.T) {
	e := newTestEnv(t)

	cases := []struct {
		name   string
		method string
		path   string
		role   string
		body   string
		want   int
	}{
		{"viewer GET agents", "GET", "/api/v1/agents", "viewer", "", 200},
		{"viewer POST agents forbidden", "POST", "/api/v1/agents", "viewer", `{"name":"x"}`, 403},
		{"viewer DELETE agents forbidden", "DELETE", "/api/v1/agents/a1", "viewer", "", 403},
		{"operator GET agents", "GET", "/api/v1/agents", "operator", "", 200},
		{"operator POST agents forbidden", "POST", "/api/v1/agents", "operator", `{"name":"x"}`, 403},
		{"operator POST tasks allowed", "POST", "/api/v1/tasks", "operator", `{"task":"hello"}`, 200},
		{"operator PUT agents forbidden", "PUT", "/api/v1/agents/a1", "operator", `{"name":"x"}`, 403},
		{"owner POST agents allowed", "POST", "/api/v1/agents", "owner", `{"name":"x"}`, 201},
		{"owner PUT agents", "PUT", "/api/v1/agents/a1", "owner", `{"name":"x"}`, 404},
		{"owner DELETE agents", "DELETE", "/api/v1/agents/a1", "owner", "", 404},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := e.token(t, tc.role)
			resp, _ := e.do(t, tc.method, tc.path, tc.body, tok)
			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestNoTokenRejected(t *testing.T) {
	e := newTestEnv(t)
	resp, _ := e.do(t, http.MethodGet, "/api/v1/agents", "", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAgentsCRUD(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	// Create
	resp, body := e.do(t, http.MethodPost, "/api/v1/agents", `{"name":"weather","profile":"p","system_prompt":"sp","skills":[{"name":"weather","server_id":"s","tool":"t"}],"enabled":true}`, tok)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", resp.StatusCode, body)
	}
	var agent store.Agent
	if err := json.Unmarshal([]byte(body), &agent); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if agent.ID == "" || agent.TenantID != defaultTenantID {
		t.Fatalf("agent = %+v", agent)
	}

	// List
	resp, body = e.do(t, http.MethodGet, "/api/v1/agents", "", tok)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "weather") {
		t.Fatalf("list status = %d, body = %s", resp.StatusCode, body)
	}

	// Get
	resp, body = e.do(t, http.MethodGet, "/api/v1/agents/"+agent.ID, "", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", resp.StatusCode)
	}

	// Update
	resp, body = e.do(t, http.MethodPut, "/api/v1/agents/"+agent.ID, `{"name":"weather2","enabled":false}`, tok)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "weather2") {
		t.Fatalf("update status = %d, body = %s", resp.StatusCode, body)
	}

	// Delete
	resp, _ = e.do(t, http.MethodDelete, "/api/v1/agents/"+agent.ID, "", tok)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
}

func TestFlowCreateValidateAndRun(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	flowJSON := `{
		"nodes":[
			{"id":"t","type":"trigger"},
			{"id":"o","type":"output","kind":"database","target":"db1","value":"v"}
		],
		"edges":[{"source":"t","target":"o"}]
	}`
	createBody := `{"name":"f1","flow_json":` + flowJSON + `,"permissions":{"databases":["db1"]},"enabled":true}`

	// Invalid flow should be rejected.
	resp, _ := e.do(t, http.MethodPost, "/api/v1/flows", `{"name":"bad","flow_json":{"nodes":[]}}`, tok)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid flow create status = %d, want 400", resp.StatusCode)
	}

	resp, body := e.do(t, http.MethodPost, "/api/v1/flows", createBody, tok)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create flow status = %d, body = %s", resp.StatusCode, body)
	}
	var fl store.Flow
	if err := json.Unmarshal([]byte(body), &fl); err != nil {
		t.Fatalf("unmarshal flow: %v", err)
	}

	// Run
	resp, body = e.do(t, http.MethodPost, "/api/v1/flows/"+fl.ID+"/run", "", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", resp.StatusCode, body)
	}
	var runResp struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(body), &runResp); err != nil || runResp.RunID == "" {
		t.Fatalf("run response %q", body)
	}

	// Get run
	resp, body = e.do(t, http.MethodGet, "/api/v1/runs/"+runResp.RunID, "", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get run status = %d", resp.StatusCode)
	}
	var run store.Run
	if err := json.Unmarshal([]byte(body), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if run.Status != store.RunStatusSuccess {
		t.Fatalf("run status = %q, want success (result=%s)", run.Status, run.Result)
	}

	// List runs for flow
	resp, body = e.do(t, http.MethodGet, "/api/v1/flows/"+fl.ID+"/runs", "", tok)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, runResp.RunID) {
		t.Fatalf("list runs status = %d, body = %s", resp.StatusCode, body)
	}
}

func TestTaskCreateAndGet(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	resp, body := e.do(t, http.MethodPost, "/api/v1/tasks", `{"task":"do something"}`, tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create task status = %d, body = %s", resp.StatusCode, body)
	}
	var respBody struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(body), &respBody); err != nil || respBody.TaskID == "" {
		t.Fatalf("task response %q", body)
	}

	resp, body = e.do(t, http.MethodGet, "/api/v1/tasks/"+respBody.TaskID, "", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get task status = %d", resp.StatusCode)
	}
	var task store.Task
	if err := json.Unmarshal([]byte(body), &task); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if task.Input != "do something" {
		t.Fatalf("task input = %q", task.Input)
	}
}

func TestDashboard(t *testing.T) {
	e := newTestEnv(t)
	tok := e.token(t, "owner")

	resp, body := e.do(t, http.MethodGet, "/api/v1/dashboard", "", tok)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "agents") {
		t.Fatalf("dashboard body = %s", body)
	}
}

func TestHealthAndMetrics(t *testing.T) {
	e := newTestEnv(t)
	resp, _ := e.do(t, http.MethodGet, "/healthz", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
	resp, _ = e.do(t, http.MethodGet, "/readyz", "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz = %d", resp.StatusCode)
	}
	resp, body := e.do(t, http.MethodGet, "/metrics", "", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "http_requests_total") {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
}
