package store

import (
	"encoding/json"
	"time"
)

// Skill describes a single capability of an Agent, mapping a tool exposed by
// an MCP server.
type Skill struct {
	Name     string `json:"name"`
	ServerID string `json:"server_id"`
	Tool     string `json:"tool"`
}

// Agent is an autonomous unit that executes subtasks using its MCP-backed
// skills.
type Agent struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Profile      string    `json:"profile"`
	SystemPrompt string    `json:"system_prompt"`
	Skills       []Skill   `json:"skills"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// McpServer is a registered Model Context Protocol server, reachable over the
// transport encoded in Transport.
type McpServer struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Name      string          `json:"name"`
	Transport json.RawMessage `json:"transport"`
	Enabled   bool            `json:"enabled"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// FlowPermissionSet is embedded in Flow to bound the side effects a flow may
// perform.
type FlowPermissionSet struct {
	Resources []string `json:"resources"`
	Files     []string `json:"files"`
	Databases []string `json:"databases"`
}

// Flow is a declarative workflow described by FlowJSON (see internal/flow).
type Flow struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	Name        string            `json:"name"`
	FlowJSON    json.RawMessage   `json:"flow_json"`
	Permissions FlowPermissionSet `json:"permissions"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// LogEntry is a single structured log line captured during a Run or Task.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
}

// RunStatus enumerates the lifecycle states of a Run.
type RunStatus string

const (
	RunStatusPending    RunStatus = "pending"
	RunStatusRunning    RunStatus = "running"
	RunStatusSuccess    RunStatus = "success"
	RunStatusFailed     RunStatus = "failed"
	RunStatusRolledBack RunStatus = "rolled_back"
)

// Run is a single execution of a Flow.
type Run struct {
	ID         string          `json:"id"`
	FlowID     string          `json:"flow_id"`
	TenantID   string          `json:"tenant_id"`
	Status     RunStatus       `json:"status"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Logs       []LogEntry      `json:"logs"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// TaskStatus enumerates the lifecycle states of a Task.
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
)

// SubtaskStatus enumerates the lifecycle states of a Subtask.
type SubtaskStatus string

const (
	SubtaskStatusPending SubtaskStatus = "pending"
	SubtaskStatusRunning SubtaskStatus = "running"
	SubtaskStatusSuccess SubtaskStatus = "success"
	SubtaskStatusFailed  SubtaskStatus = "failed"
)

// Subtask is a single decomposed unit of work assigned to one Agent.
type Subtask struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	Description string          `json:"description"`
	Status      SubtaskStatus   `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// Task is a global, decomposed unit of work distributed across Agents.
type Task struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	Input     string          `json:"input"`
	Status    TaskStatus      `json:"status"`
	Subtasks  []Subtask       `json:"subtasks"`
	Result    json.RawMessage `json:"result,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Role enumerates the RBAC roles supported by the platform.
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleOwner    Role = "owner"
)

// User is an authenticated principal belonging to a Tenant.
type User struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Tenant is the top-level isolation boundary for all resources.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}
