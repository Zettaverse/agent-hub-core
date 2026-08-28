-- 0001_init.sql — full schema for the agent-hub-core platform.
-- All tables are tenant-scoped and use UUID primary keys.

CREATE TABLE IF NOT EXISTS tenants (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'viewer',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, username)
);

CREATE TABLE IF NOT EXISTS agents (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    profile       TEXT NOT NULL DEFAULT '',
    system_prompt TEXT NOT NULL DEFAULT '',
    skills        JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled       BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agents_tenant ON agents(tenant_id);

CREATE TABLE IF NOT EXISTS mcp_servers (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    transport   JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    status      TEXT NOT NULL DEFAULT 'unknown',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_tenant ON mcp_servers(tenant_id);

CREATE TABLE IF NOT EXISTS flows (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    flow_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    permissions JSONB NOT NULL DEFAULT '{"resources":[],"files":[],"databases":[]}'::jsonb,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_flows_tenant ON flows(tenant_id);

CREATE TABLE IF NOT EXISTS runs (
    id          UUID PRIMARY KEY,
    flow_id     UUID NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    logs        JSONB NOT NULL DEFAULT '[]'::jsonb,
    result      JSONB
);
CREATE INDEX IF NOT EXISTS idx_runs_tenant ON runs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_runs_flow ON runs(flow_id);

CREATE TABLE IF NOT EXISTS tasks (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    input       TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    subtasks    JSONB NOT NULL DEFAULT '[]'::jsonb,
    result      JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant ON tasks(tenant_id);
