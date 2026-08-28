# agent-hub-core

Production-grade Go backend for the Zettaverse agent hub: multi-tenant agent
orchestration, MCP tool execution, declarative flow automation with rollback,
RBAC, live WebSocket activity feed, and Prometheus metrics.

## Features

- **Agents & MCP servers** — CRUD for agents (with skills) and MCP servers
  (stdio / websocket transports).
- **Task Distribution Engine** — decomposes a global task into subtasks by
  keyword/skill overlap, assigns each to the best-matching enabled agent, and
  executes them concurrently with a bounded worker pool and per-subtask
  timeouts.
- **Flow engine** — interprets `flow_json` graphs (trigger / agent / mcp_tool /
  condition / output nodes), validates the graph (single trigger, acyclic, no
  dangling edges), executes deterministically, evaluates a tiny embedded
  expression language for conditions, enforces a permissions boundary for
  side-effecting output nodes, and rolls back side effects on failure.
- **MCP client** — JSON-RPC 2.0 over stdio and websocket (pure Go, default) or
  via the Rust `libmcpengine` C ABI (build tag `mcpengine`).
- **Auth & RBAC** — HS256 JWTs; `viewer` (read), `operator` (read + run
  flows/tasks), `owner` (everything incl. user management).
- **WebSocket hub** — per-tenant rooms broadcasting `agent_message`,
  `run_update`, and `task_update` events.
- **Observability** — `log/slog` JSON logging, request-ID + logging +
  recovery middleware, Prometheus metrics at `/metrics`.

## Requirements

- Go 1.26
- PostgreSQL 14+ (production) — or run with `USE_MEMORY_STORE=true`
- (Optional) Rust + `cargo` for the `mcpengine` build tag

## Quick start

```sh
# In-memory store, no external dependencies:
USE_MEMORY_STORE=true JWT_SECRET=change-me go run ./cmd/hub
```

Default seed: an `owner` account `admin` / `admin` is created on first login
(override with `SEED_OWNER_USERNAME` / `SEED_OWNER_PASSWORD`).

```sh
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
# {"token":"<jwt>"}
```

Use the token as `Authorization: Bearer <jwt>`.

## Configuration

| Variable               | Default   | Description                              |
| ---------------------- | --------- | ---------------------------------------- |
| `PORT`                 | `8080`    | HTTP listen port                         |
| `HOST`                 | `0.0.0.0` | HTTP listen host                         |
| `DATABASE_URL`         | _(empty)_ | PostgreSQL DSN (required unless memory)  |
| `USE_MEMORY_STORE`     | `false`   | Use the in-memory store                  |
| `JWT_SECRET`           | _(empty)_ | HMAC secret (required unless memory)     |
| `JWT_EXPIRY`           | `24h`     | Token lifetime                           |
| `LOG_LEVEL`            | `info`    | `debug`/`info`/`warn`/`error`            |
| `SEED_OWNER_USERNAME`  | `admin`   | Default owner username                   |
| `SEED_OWNER_PASSWORD`  | `admin`   | Default owner password                   |
| `TASK_WORKER_POOL`     | `4`       | Concurrent subtask workers               |
| `TASK_TIMEOUT`         | `30s`     | Per-subtask timeout                      |

## Build & test

```sh
go build ./...
go test -race ./...
go vet ./...
gofmt -l .
```

The default build uses the **pure-Go** MCP client and requires **no cgo and no
Rust**. To build against the Rust engine:

```sh
# Build the Rust cdylib first (see Dockerfile), then:
CGO_ENABLED=1 go build -tags mcpengine ./cmd/hub
```

On Windows the `-race` flag needs a C toolchain (e.g. MinGW-w64); on Linux CI
this is provided by the runner.

## Architecture

```
cmd/hub                 entrypoint (config, slog, chi, graceful shutdown)
internal/config         env-based configuration
internal/logging        slog JSON logger
internal/store          Store interface + in-memory + pgx impls + migrations
internal/auth           JWT issue/verify + middleware + RBAC
internal/api            chi router + handlers
internal/mcp            MCP client (pure-Go | cgo) + JSON-RPC 2.0
internal/orchestrator   task decomposition + distribution
internal/flow           flow_json interpreter (graph, nodes, expr, rollback)
internal/ws             WebSocket hub (per-tenant rooms)
internal/metrics        Prometheus collectors
migrations              embedded SQL schema
openapi                 OpenAPI 3.0 specification
```

### Data model

`Agent` (uuid, tenant, name, profile, system_prompt, skills[], enabled),
`McpServer` (uuid, tenant, name, transport, enabled, status), `Flow` (uuid,
tenant, name, flow_json, permissions, enabled), `Run` (uuid, flow, tenant,
status, started/finished, logs[], result), `Task` (uuid, tenant, input,
status, subtasks[], result), `User` (uuid, tenant, username, bcrypt hash,
role), `Tenant` (uuid, name).

### RBAC matrix

| Role       | Capabilities                                                        |
| ---------- | ------------------------------------------------------------------- |
| `viewer`   | `GET` on all `/api/v1/*`                                            |
| `operator` | `viewer` + `POST /api/v1/tasks`, `POST /api/v1/flows/{id}/run`      |
| `owner`    | everything, including `POST/PUT/DELETE` of resources and user mgmt   |

### Flow JSON

```json
{
  "nodes": [
    { "id": "t",  "type": "trigger", "value": { "value": 20 } },
    { "id": "c",  "type": "condition", "expression": "result.value > 10" },
    { "id": "o",  "type": "output", "kind": "database", "target": "db1", "value": "yes" }
  ],
  "edges": [
    { "source": "t", "target": "c" },
    { "source": "c", "source_handle": "true", "target": "o" }
  ]
}
```

Conditions support `== != > < >= <= && || !`, `.length`, string/number/bool
literals, and `result` / `result.field` access. Side-effecting `output` nodes
(`database`/`file`/`modbus`) must match `flow.permissions`; on any node error,
side effects are rolled back in reverse order and the run is marked
`rolled_back`.

## Docker

```sh
docker build -t agent-hub-core .
docker run -p 8080:8080 -e DATABASE_URL=... -e JWT_SECRET=... agent-hub-core
```

The multi-stage build clones `Zettaverse/agent-hub-mcp`, builds the Rust
cdylib, then compiles the Go binary with `-tags mcpengine` and `CGO_ENABLED=1`,
producing a minimal distroless image.

## API

Full OpenAPI 3.0 spec at [`openapi/openapi.yaml`](openapi/openapi.yaml).

Key routes:

```
POST   /api/v1/auth/login
GET    /api/v1/me
GET    /api/v1/agents            POST   /api/v1/agents
GET    /api/v1/agents/{id}       PUT/DELETE /api/v1/agents/{id}
GET    /api/v1/mcp-servers       POST   /api/v1/mcp-servers
GET    /api/v1/mcp-servers/{id}  PUT/DELETE /api/v1/mcp-servers/{id}
POST   /api/v1/mcp-servers/{id}/test
GET    /api/v1/flows             POST   /api/v1/flows
GET    /api/v1/flows/{id}        PUT/DELETE /api/v1/flows/{id}
POST   /api/v1/flows/{id}/run    GET    /api/v1/flows/{id}/runs
GET    /api/v1/runs/{run_id}
POST   /api/v1/tasks             GET    /api/v1/tasks/{id}
GET    /api/v1/users             POST   /api/v1/users
GET    /api/v1/users/{id}        PUT/DELETE /api/v1/users/{id}
GET    /api/v1/dashboard
WS     /api/v1/ws?token=...
GET    /healthz   /readyz   /metrics
```

## License

MIT © 2026 Zettaverse
