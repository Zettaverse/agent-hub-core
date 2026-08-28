package store

import (
	"context"
	"sort"
	"sync"
)

// Memory is a thread-safe in-memory Store implementation, primarily intended
// for tests and local development. All access is guarded by a single
// sync.RWMutex.
type Memory struct {
	mu      sync.RWMutex
	agents  map[string]map[string]Agent     // tenantID -> id -> Agent
	servers map[string]map[string]McpServer // tenantID -> id -> McpServer
	flows   map[string]map[string]Flow      // tenantID -> id -> Flow
	runs    map[string]map[string]Run       // tenantID -> id -> Run
	tasks   map[string]map[string]Task      // tenantID -> id -> Task
	users   map[string]map[string]User      // tenantID -> id -> User
	tenants map[string]Tenant               // id -> Tenant
}

func NewMemoryStore() *Memory {
	return &Memory{
		agents:  make(map[string]map[string]Agent),
		servers: make(map[string]map[string]McpServer),
		flows:   make(map[string]map[string]Flow),
		runs:    make(map[string]map[string]Run),
		tasks:   make(map[string]map[string]Task),
		users:   make(map[string]map[string]User),
		tenants: make(map[string]Tenant),
	}
}

func (m *Memory) Close() {}

func (m *Memory) tenantMap(tenantID string) map[string]Agent {
	if m.agents[tenantID] == nil {
		m.agents[tenantID] = make(map[string]Agent)
	}
	return m.agents[tenantID]
}

// --- Agents ---

func (m *Memory) ListAgents(ctx context.Context, tenantID string) ([]Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.agents[tenantID]
	out := make([]Agent, 0, len(tm))
	for _, a := range tm {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetAgent(ctx context.Context, tenantID, id string) (Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if a, ok := m.agents[tenantID][id]; ok {
		return a, nil
	}
	return Agent{}, ErrNotFound
}

func (m *Memory) CreateAgent(ctx context.Context, agent Agent) (Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if agent.ID == "" {
		return Agent{}, ErrInvalid
	}
	tm := m.agentMap(agent.TenantID)
	if _, exists := tm[agent.ID]; exists {
		return Agent{}, ErrConflict
	}
	tm[agent.ID] = agent
	return agent, nil
}

func (m *Memory) UpdateAgent(ctx context.Context, agent Agent) (Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.agentMap(agent.TenantID)
	if _, exists := tm[agent.ID]; !exists {
		return Agent{}, ErrNotFound
	}
	tm[agent.ID] = agent
	return agent, nil
}

func (m *Memory) DeleteAgent(ctx context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.agents[tenantID][id]; !exists {
		return ErrNotFound
	}
	delete(m.agents[tenantID], id)
	return nil
}

func (m *Memory) agentMap(tenantID string) map[string]Agent {
	if m.agents[tenantID] == nil {
		m.agents[tenantID] = make(map[string]Agent)
	}
	return m.agents[tenantID]
}

// --- McpServers ---

func (m *Memory) ListMcpServers(ctx context.Context, tenantID string) ([]McpServer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.servers[tenantID]
	out := make([]McpServer, 0, len(tm))
	for _, s := range tm {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetMcpServer(ctx context.Context, tenantID, id string) (McpServer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.servers[tenantID][id]; ok {
		return s, nil
	}
	return McpServer{}, ErrNotFound
}

func (m *Memory) CreateMcpServer(ctx context.Context, server McpServer) (McpServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if server.ID == "" {
		return McpServer{}, ErrInvalid
	}
	tm := m.serverMap(server.TenantID)
	if _, exists := tm[server.ID]; exists {
		return McpServer{}, ErrConflict
	}
	tm[server.ID] = server
	return server, nil
}

func (m *Memory) UpdateMcpServer(ctx context.Context, server McpServer) (McpServer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.serverMap(server.TenantID)
	if _, exists := tm[server.ID]; !exists {
		return McpServer{}, ErrNotFound
	}
	tm[server.ID] = server
	return server, nil
}

func (m *Memory) DeleteMcpServer(ctx context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.servers[tenantID][id]; !exists {
		return ErrNotFound
	}
	delete(m.servers[tenantID], id)
	return nil
}

func (m *Memory) serverMap(tenantID string) map[string]McpServer {
	if m.servers[tenantID] == nil {
		m.servers[tenantID] = make(map[string]McpServer)
	}
	return m.servers[tenantID]
}

// --- Flows ---

func (m *Memory) ListFlows(ctx context.Context, tenantID string) ([]Flow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.flows[tenantID]
	out := make([]Flow, 0, len(tm))
	for _, f := range tm {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) GetFlow(ctx context.Context, tenantID, id string) (Flow, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if f, ok := m.flows[tenantID][id]; ok {
		return f, nil
	}
	return Flow{}, ErrNotFound
}

func (m *Memory) CreateFlow(ctx context.Context, flow Flow) (Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if flow.ID == "" {
		return Flow{}, ErrInvalid
	}
	tm := m.flowMap(flow.TenantID)
	if _, exists := tm[flow.ID]; exists {
		return Flow{}, ErrConflict
	}
	tm[flow.ID] = flow
	return flow, nil
}

func (m *Memory) UpdateFlow(ctx context.Context, flow Flow) (Flow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.flowMap(flow.TenantID)
	if _, exists := tm[flow.ID]; !exists {
		return Flow{}, ErrNotFound
	}
	tm[flow.ID] = flow
	return flow, nil
}

func (m *Memory) DeleteFlow(ctx context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.flows[tenantID][id]; !exists {
		return ErrNotFound
	}
	delete(m.flows[tenantID], id)
	return nil
}

func (m *Memory) flowMap(tenantID string) map[string]Flow {
	if m.flows[tenantID] == nil {
		m.flows[tenantID] = make(map[string]Flow)
	}
	return m.flows[tenantID]
}

// --- Runs ---

func (m *Memory) CreateRun(ctx context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if run.ID == "" {
		return Run{}, ErrInvalid
	}
	tm := m.runMap(run.TenantID)
	if _, exists := tm[run.ID]; exists {
		return Run{}, ErrConflict
	}
	tm[run.ID] = run
	return run, nil
}

func (m *Memory) GetRun(ctx context.Context, tenantID, id string) (Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.runs[tenantID][id]; ok {
		return r, nil
	}
	return Run{}, ErrNotFound
}

func (m *Memory) ListRuns(ctx context.Context, tenantID, flowID string) ([]Run, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.runs[tenantID]
	out := make([]Run, 0, len(tm))
	for _, r := range tm {
		if flowID != "" && r.FlowID != flowID {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out, nil
}

func (m *Memory) UpdateRun(ctx context.Context, run Run) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.runMap(run.TenantID)
	if _, exists := tm[run.ID]; !exists {
		return Run{}, ErrNotFound
	}
	tm[run.ID] = run
	return run, nil
}

func (m *Memory) runMap(tenantID string) map[string]Run {
	if m.runs[tenantID] == nil {
		m.runs[tenantID] = make(map[string]Run)
	}
	return m.runs[tenantID]
}

// --- Tasks ---

func (m *Memory) CreateTask(ctx context.Context, task Task) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task.ID == "" {
		return Task{}, ErrInvalid
	}
	tm := m.taskMap(task.TenantID)
	if _, exists := tm[task.ID]; exists {
		return Task{}, ErrConflict
	}
	tm[task.ID] = task
	return task, nil
}

func (m *Memory) GetTask(ctx context.Context, tenantID, id string) (Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tasks[tenantID][id]; ok {
		return t, nil
	}
	return Task{}, ErrNotFound
}

func (m *Memory) ListTasks(ctx context.Context, tenantID string) ([]Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.tasks[tenantID]
	out := make([]Task, 0, len(tm))
	for _, t := range tm {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *Memory) UpdateTask(ctx context.Context, task Task) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.taskMap(task.TenantID)
	if _, exists := tm[task.ID]; !exists {
		return Task{}, ErrNotFound
	}
	tm[task.ID] = task
	return task, nil
}

func (m *Memory) taskMap(tenantID string) map[string]Task {
	if m.tasks[tenantID] == nil {
		m.tasks[tenantID] = make(map[string]Task)
	}
	return m.tasks[tenantID]
}

// --- Users ---

func (m *Memory) ListUsers(ctx context.Context, tenantID string) ([]User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tm := m.users[tenantID]
	out := make([]User, 0, len(tm))
	for _, u := range tm {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

func (m *Memory) GetUser(ctx context.Context, tenantID, id string) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if u, ok := m.users[tenantID][id]; ok {
		return u, nil
	}
	return User{}, ErrNotFound
}

func (m *Memory) GetUserByUsername(ctx context.Context, tenantID, username string) (User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users[tenantID] {
		if u.Username == username {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func (m *Memory) CreateUser(ctx context.Context, user User) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if user.ID == "" {
		return User{}, ErrInvalid
	}
	tm := m.userMap(user.TenantID)
	if _, exists := tm[user.ID]; exists {
		return User{}, ErrConflict
	}
	tm[user.ID] = user
	return user, nil
}

func (m *Memory) UpdateUser(ctx context.Context, user User) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tm := m.userMap(user.TenantID)
	if _, exists := tm[user.ID]; !exists {
		return User{}, ErrNotFound
	}
	tm[user.ID] = user
	return user, nil
}

func (m *Memory) DeleteUser(ctx context.Context, tenantID, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.users[tenantID][id]; !exists {
		return ErrNotFound
	}
	delete(m.users[tenantID], id)
	return nil
}

func (m *Memory) userMap(tenantID string) map[string]User {
	if m.users[tenantID] == nil {
		m.users[tenantID] = make(map[string]User)
	}
	return m.users[tenantID]
}

// --- Tenants ---

func (m *Memory) GetTenant(ctx context.Context, id string) (Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tenants[id]; ok {
		return t, nil
	}
	return Tenant{}, ErrNotFound
}

func (m *Memory) CreateTenant(ctx context.Context, tenant Tenant) (Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tenant.ID == "" {
		return Tenant{}, ErrInvalid
	}
	if _, exists := m.tenants[tenant.ID]; exists {
		return Tenant{}, ErrConflict
	}
	m.tenants[tenant.ID] = tenant
	return tenant, nil
}
