package store

import (
	"context"
	"errors"
)

// Sentinel errors returned by Store implementations.
var (
	ErrNotFound     = errors.New("store: not found")
	ErrConflict     = errors.New("store: conflict")
	ErrInvalid      = errors.New("store: invalid input")
	ErrUnauthorized = errors.New("store: unauthorized")
)

// Store is the persistence abstraction used across the platform. The
// in-memory implementation (Memory) backs all tests; the pgx implementation
// (PG) backs production. Every mutating operation takes the acting tenant so
// that cross-tenant access can be rejected at the storage layer.
type Store interface {
	AgentStore
	McpServerStore
	FlowStore
	RunStore
	TaskStore
	UserStore
	TenantStore

	// Close releases any underlying resources (e.g. a connection pool).
	Close()
}

// AgentStore manages Agent resources.
type AgentStore interface {
	ListAgents(ctx context.Context, tenantID string) ([]Agent, error)
	GetAgent(ctx context.Context, tenantID, id string) (Agent, error)
	CreateAgent(ctx context.Context, agent Agent) (Agent, error)
	UpdateAgent(ctx context.Context, agent Agent) (Agent, error)
	DeleteAgent(ctx context.Context, tenantID, id string) error
}

// McpServerStore manages McpServer resources.
type McpServerStore interface {
	ListMcpServers(ctx context.Context, tenantID string) ([]McpServer, error)
	GetMcpServer(ctx context.Context, tenantID, id string) (McpServer, error)
	CreateMcpServer(ctx context.Context, server McpServer) (McpServer, error)
	UpdateMcpServer(ctx context.Context, server McpServer) (McpServer, error)
	DeleteMcpServer(ctx context.Context, tenantID, id string) error
}

// FlowStore manages Flow resources.
type FlowStore interface {
	ListFlows(ctx context.Context, tenantID string) ([]Flow, error)
	GetFlow(ctx context.Context, tenantID, id string) (Flow, error)
	CreateFlow(ctx context.Context, flow Flow) (Flow, error)
	UpdateFlow(ctx context.Context, flow Flow) (Flow, error)
	DeleteFlow(ctx context.Context, tenantID, id string) error
}

// RunStore manages Run resources.
type RunStore interface {
	CreateRun(ctx context.Context, run Run) (Run, error)
	GetRun(ctx context.Context, tenantID, id string) (Run, error)
	ListRuns(ctx context.Context, tenantID, flowID string) ([]Run, error)
	UpdateRun(ctx context.Context, run Run) (Run, error)
}

// TaskStore manages Task resources.
type TaskStore interface {
	CreateTask(ctx context.Context, task Task) (Task, error)
	GetTask(ctx context.Context, tenantID, id string) (Task, error)
	ListTasks(ctx context.Context, tenantID string) ([]Task, error)
	UpdateTask(ctx context.Context, task Task) (Task, error)
}

// UserStore manages User resources.
type UserStore interface {
	ListUsers(ctx context.Context, tenantID string) ([]User, error)
	GetUser(ctx context.Context, tenantID, id string) (User, error)
	GetUserByUsername(ctx context.Context, tenantID, username string) (User, error)
	CreateUser(ctx context.Context, user User) (User, error)
	UpdateUser(ctx context.Context, user User) (User, error)
	DeleteUser(ctx context.Context, tenantID, id string) error
}

// TenantStore manages Tenant resources.
type TenantStore interface {
	GetTenant(ctx context.Context, id string) (Tenant, error)
	CreateTenant(ctx context.Context, tenant Tenant) (Tenant, error)
}
