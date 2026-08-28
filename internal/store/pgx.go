package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PG is the production Store implementation backed by PostgreSQL via pgxpool.
type PG struct {
	pool *pgxpool.Pool
}

// NewPG connects to PostgreSQL and returns a ready-to-use Store. It does not
// run migrations; call Migrate explicitly (or use NewPGWithMigrations).
func NewPG(ctx context.Context, databaseURL string) (*PG, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &PG{pool: pool}, nil
}

// NewPGWithMigrations connects and applies embedded migrations before
// returning the Store.
func NewPGWithMigrations(ctx context.Context, databaseURL string) (*PG, error) {
	pg, err := NewPG(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pg.Migrate(ctx); err != nil {
		pg.Close()
		return nil, err
	}
	return pg, nil
}

// Migrate applies all embedded SQL migrations in order.
func (p *PG) Migrate(ctx context.Context) error {
	statements, err := loadMigrationStatements()
	if err != nil {
		return err
	}
	for _, stmt := range statements {
		if _, err := p.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("store: migrate: %w", err)
		}
	}
	return nil
}

func (p *PG) Close() {
	p.pool.Close()
}

// --- Agents ---

func (p *PG) ListAgents(ctx context.Context, tenantID string) ([]Agent, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, tenant_id, name, profile, system_prompt, skills, enabled, created_at, updated_at FROM agents WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		var a Agent
		if err := scanAgent(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p *PG) GetAgent(ctx context.Context, tenantID, id string) (Agent, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, name, profile, system_prompt, skills, enabled, created_at, updated_at FROM agents WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	var a Agent
	if err := scanAgent(row, &a); err != nil {
		return Agent{}, wrapNotFound(err)
	}
	return a, nil
}

func (p *PG) CreateAgent(ctx context.Context, agent Agent) (Agent, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO agents (id, tenant_id, name, profile, system_prompt, skills, enabled, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		agent.ID, agent.TenantID, agent.Name, agent.Profile, agent.SystemPrompt, skillsJSON(agent.Skills), agent.Enabled, agent.CreatedAt, agent.UpdatedAt)
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (p *PG) UpdateAgent(ctx context.Context, agent Agent) (Agent, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE agents SET name=$1, profile=$2, system_prompt=$3, skills=$4, enabled=$5, updated_at=$6 WHERE tenant_id=$7 AND id=$8`,
		agent.Name, agent.Profile, agent.SystemPrompt, skillsJSON(agent.Skills), agent.Enabled, agent.UpdatedAt, agent.TenantID, agent.ID)
	if err != nil {
		return Agent{}, err
	}
	if tag.RowsAffected() == 0 {
		return Agent{}, ErrNotFound
	}
	return agent, nil
}

func (p *PG) DeleteAgent(ctx context.Context, tenantID, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM agents WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- McpServers ---

func (p *PG) ListMcpServers(ctx context.Context, tenantID string) ([]McpServer, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, tenant_id, name, transport, enabled, status, created_at, updated_at FROM mcp_servers WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []McpServer
	for rows.Next() {
		var s McpServer
		if err := scanMcpServer(rows, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *PG) GetMcpServer(ctx context.Context, tenantID, id string) (McpServer, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, name, transport, enabled, status, created_at, updated_at FROM mcp_servers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var s McpServer
	if err := scanMcpServer(row, &s); err != nil {
		return McpServer{}, wrapNotFound(err)
	}
	return s, nil
}

func (p *PG) CreateMcpServer(ctx context.Context, server McpServer) (McpServer, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO mcp_servers (id, tenant_id, name, transport, enabled, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		server.ID, server.TenantID, server.Name, server.Transport, server.Enabled, server.Status, server.CreatedAt, server.UpdatedAt)
	if err != nil {
		return McpServer{}, err
	}
	return server, nil
}

func (p *PG) UpdateMcpServer(ctx context.Context, server McpServer) (McpServer, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE mcp_servers SET name=$1, transport=$2, enabled=$3, status=$4, updated_at=$5 WHERE tenant_id=$6 AND id=$7`,
		server.Name, server.Transport, server.Enabled, server.Status, server.UpdatedAt, server.TenantID, server.ID)
	if err != nil {
		return McpServer{}, err
	}
	if tag.RowsAffected() == 0 {
		return McpServer{}, ErrNotFound
	}
	return server, nil
}

func (p *PG) DeleteMcpServer(ctx context.Context, tenantID, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Flows ---

func (p *PG) ListFlows(ctx context.Context, tenantID string) ([]Flow, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, tenant_id, name, flow_json, permissions, enabled, created_at, updated_at FROM flows WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Flow
	for rows.Next() {
		var f Flow
		if err := scanFlow(rows, &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (p *PG) GetFlow(ctx context.Context, tenantID, id string) (Flow, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, name, flow_json, permissions, enabled, created_at, updated_at FROM flows WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var f Flow
	if err := scanFlow(row, &f); err != nil {
		return Flow{}, wrapNotFound(err)
	}
	return f, nil
}

func (p *PG) CreateFlow(ctx context.Context, flow Flow) (Flow, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO flows (id, tenant_id, name, flow_json, permissions, enabled, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		flow.ID, flow.TenantID, flow.Name, flow.FlowJSON, permissionsJSON(flow.Permissions), flow.Enabled, flow.CreatedAt, flow.UpdatedAt)
	if err != nil {
		return Flow{}, err
	}
	return flow, nil
}

func (p *PG) UpdateFlow(ctx context.Context, flow Flow) (Flow, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE flows SET name=$1, flow_json=$2, permissions=$3, enabled=$4, updated_at=$5 WHERE tenant_id=$6 AND id=$7`,
		flow.Name, flow.FlowJSON, permissionsJSON(flow.Permissions), flow.Enabled, flow.UpdatedAt, flow.TenantID, flow.ID)
	if err != nil {
		return Flow{}, err
	}
	if tag.RowsAffected() == 0 {
		return Flow{}, ErrNotFound
	}
	return flow, nil
}

func (p *PG) DeleteFlow(ctx context.Context, tenantID, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM flows WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Runs ---

func (p *PG) CreateRun(ctx context.Context, run Run) (Run, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO runs (id, flow_id, tenant_id, status, started_at, finished_at, logs, result) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		run.ID, run.FlowID, run.TenantID, string(run.Status), run.StartedAt, run.FinishedAt, logsJSON(run.Logs), run.Result)
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

func (p *PG) GetRun(ctx context.Context, tenantID, id string) (Run, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, flow_id, tenant_id, status, started_at, finished_at, logs, result FROM runs WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var r Run
	var status string
	var finishedAt *time.Time
	if err := row.Scan(&r.ID, &r.FlowID, &r.TenantID, &status, &r.StartedAt, &finishedAt, &r.Logs, &r.Result); err != nil {
		return Run{}, wrapNotFound(err)
	}
	r.Status = RunStatus(status)
	r.FinishedAt = finishedAt
	return r, nil
}

func (p *PG) ListRuns(ctx context.Context, tenantID, flowID string) ([]Run, error) {
	query := `SELECT id, flow_id, tenant_id, status, started_at, finished_at, logs, result FROM runs WHERE tenant_id=$1`
	args := []any{tenantID}
	if flowID != "" {
		query += ` AND flow_id=$2`
		args = append(args, flowID)
	}
	query += ` ORDER BY started_at`
	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		var status string
		var finishedAt *time.Time
		if err := rows.Scan(&r.ID, &r.FlowID, &r.TenantID, &status, &r.StartedAt, &finishedAt, &r.Logs, &r.Result); err != nil {
			return nil, err
		}
		r.Status = RunStatus(status)
		r.FinishedAt = finishedAt
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *PG) UpdateRun(ctx context.Context, run Run) (Run, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE runs SET status=$1, started_at=$2, finished_at=$3, logs=$4, result=$5 WHERE tenant_id=$6 AND id=$7`,
		string(run.Status), run.StartedAt, run.FinishedAt, logsJSON(run.Logs), run.Result, run.TenantID, run.ID)
	if err != nil {
		return Run{}, err
	}
	if tag.RowsAffected() == 0 {
		return Run{}, ErrNotFound
	}
	return run, nil
}

// --- Tasks ---

func (p *PG) CreateTask(ctx context.Context, task Task) (Task, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO tasks (id, tenant_id, input, status, subtasks, result, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		task.ID, task.TenantID, task.Input, string(task.Status), subtasksJSON(task.Subtasks), task.Result, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return Task{}, err
	}
	return task, nil
}

func (p *PG) GetTask(ctx context.Context, tenantID, id string) (Task, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, input, status, subtasks, result, created_at, updated_at FROM tasks WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var t Task
	var status string
	if err := row.Scan(&t.ID, &t.TenantID, &t.Input, &status, &t.Subtasks, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Task{}, wrapNotFound(err)
	}
	t.Status = TaskStatus(status)
	return t, nil
}

func (p *PG) ListTasks(ctx context.Context, tenantID string) ([]Task, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, tenant_id, input, status, subtasks, result, created_at, updated_at FROM tasks WHERE tenant_id=$1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		var status string
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Input, &status, &t.Subtasks, &t.Result, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Status = TaskStatus(status)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *PG) UpdateTask(ctx context.Context, task Task) (Task, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE tasks SET input=$1, status=$2, subtasks=$3, result=$4, updated_at=$5 WHERE tenant_id=$6 AND id=$7`,
		task.Input, string(task.Status), subtasksJSON(task.Subtasks), task.Result, task.UpdatedAt, task.TenantID, task.ID)
	if err != nil {
		return Task{}, err
	}
	if tag.RowsAffected() == 0 {
		return Task{}, ErrNotFound
	}
	return task, nil
}

// --- Users ---

func (p *PG) ListUsers(ctx context.Context, tenantID string) ([]User, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, tenant_id, username, password_hash, role, created_at, updated_at FROM users WHERE tenant_id=$1 ORDER BY username`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := scanUser(rows, &u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (p *PG) GetUser(ctx context.Context, tenantID, id string) (User, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, username, password_hash, role, created_at, updated_at FROM users WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	var u User
	if err := scanUser(row, &u); err != nil {
		return User{}, wrapNotFound(err)
	}
	return u, nil
}

func (p *PG) GetUserByUsername(ctx context.Context, tenantID, username string) (User, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, tenant_id, username, password_hash, role, created_at, updated_at FROM users WHERE tenant_id=$1 AND username=$2`, tenantID, username)
	var u User
	if err := scanUser(row, &u); err != nil {
		return User{}, wrapNotFound(err)
	}
	return u, nil
}

func (p *PG) CreateUser(ctx context.Context, user User) (User, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO users (id, tenant_id, username, password_hash, role, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		user.ID, user.TenantID, user.Username, user.PasswordHash, string(user.Role), user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (p *PG) UpdateUser(ctx context.Context, user User) (User, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE users SET username=$1, password_hash=$2, role=$3, updated_at=$4 WHERE tenant_id=$5 AND id=$6`,
		user.Username, user.PasswordHash, string(user.Role), user.UpdatedAt, user.TenantID, user.ID)
	if err != nil {
		return User{}, err
	}
	if tag.RowsAffected() == 0 {
		return User{}, ErrNotFound
	}
	return user, nil
}

func (p *PG) DeleteUser(ctx context.Context, tenantID, id string) error {
	tag, err := p.pool.Exec(ctx, `DELETE FROM users WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Tenants ---

func (p *PG) GetTenant(ctx context.Context, id string) (Tenant, error) {
	row := p.pool.QueryRow(ctx, `SELECT id, name, created_at FROM tenants WHERE id=$1`, id)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
		return Tenant{}, wrapNotFound(err)
	}
	return t, nil
}

func (p *PG) CreateTenant(ctx context.Context, tenant Tenant) (Tenant, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO tenants (id, name, created_at) VALUES ($1,$2,$3)`, tenant.ID, tenant.Name, tenant.CreatedAt)
	if err != nil {
		return Tenant{}, err
	}
	return tenant, nil
}

// scanAgent reads a pgx.Row (or Rows) into an Agent.
type agentScanner interface {
	Scan(dest ...any) error
}

func scanAgent(s agentScanner, a *Agent) error {
	return s.Scan(&a.ID, &a.TenantID, &a.Name, &a.Profile, &a.SystemPrompt, &a.Skills, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
}

func scanMcpServer(s agentScanner, m *McpServer) error {
	return s.Scan(&m.ID, &m.TenantID, &m.Name, &m.Transport, &m.Enabled, &m.Status, &m.CreatedAt, &m.UpdatedAt)
}

func scanFlow(s agentScanner, f *Flow) error {
	return s.Scan(&f.ID, &f.TenantID, &f.Name, &f.FlowJSON, &f.Permissions, &f.Enabled, &f.CreatedAt, &f.UpdatedAt)
}

func scanUser(s agentScanner, u *User) error {
	var role string
	if err := s.Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &role, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return err
	}
	u.Role = Role(role)
	return nil
}

func wrapNotFound(err error) error {
	if err == pgx.ErrNoRows {
		return ErrNotFound
	}
	return err
}
