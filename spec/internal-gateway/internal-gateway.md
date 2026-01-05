# Sandbox0 Internal Gateway Design Specification

## Overview

Internal Gateway is the unified entry point for sandbox0, responsible for:
1. **Authentication**: Validate client identity, support API Key and JWT
2. **Request Routing**: Forward requests to internal services (Manager/Procd/Storage Proxy)
3. **Protocol Conversion**: Unified external API protocol, shield internal service differences
4. **Traffic Control**: Rate limiting, quota management, circuit breaking

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Internal Gateway Architecture                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        HTTP Server (Port: 8443)                       │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │   Sandbox   │ │   Process   │ │SandboxVolume│ │   Template  │     │  │
│  │  │   APIs      │ │   APIs      │ │   APIs      │ │   APIs      │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        Middleware Layer                               │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │    Auth     │ │   Rate      │ │   Request   │ │   Recovery  │     │  │
│  │  │  Middleware │ │   Limit     │ │   Logging   │ │  Middleware │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│              ┌─────────────────────┼─────────────────────┬───────────────┐   │
│              ▼                     ▼                     ▼               │   │
│  ┌───────────────────┐ ┌───────────────────┐ ┌─────────────────────┐     │   │
│  │      Manager      │ │      Procd        │ │   Storage Proxy     │     │   │
│  │   (Port: 8080)    │ │   (Dynamic)       │ │   (Port: 8081)     │     │   │
│  │                   │ │                   │ │                     │     │   │
│  │  - Sandbox        │ │  - Process        │ │  - SandboxVolume   │     │   │
│  │    Management     │ │    Management     │ │    Management      │     │   │
│  │  - Template       │ │  - File System    │ │  - JuiceFS Storage │     │   │
│  │    Management     │ │  - Context        │ │  - Snapshot/Restore│     │   │
│  └───────────────────┘ └───────────────────┘ └─────────────────────┘     │   │
│                                                              │             │   │
│                                                              ▼             │   │
│                                                    ┌───────────────────┐   │   │
│                                                    │    PostgreSQL      │   │   │
│                                                    │  - API Keys       │   │   │
│                                                    │  - Teams/Users    │   │   │
│                                                    │  - Quotas         │   │   │
│                                                    │  - Audit Logs     │   │   │
│                                                    │  - Sandboxes      │   │   │
│                                                    └───────────────────┘   │   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## API Routing Table

| Route | Method | Target | Description |
|-------|--------|--------|-------------|
| **Sandbox Management** ||||
| `/api/v1/sandboxes` | POST | Manager | Create sandbox |
| `/api/v1/sandboxes` | GET | Manager | List sandboxes |
| `/api/v1/sandboxes/{id}` | GET | Manager | Get sandbox |
| `/api/v1/sandboxes/{id}` | PATCH | Manager | Update sandbox |
| `/api/v1/sandboxes/{id}` | DELETE | Manager | Delete sandbox |
| `/api/v1/sandboxes/{id}/status` | GET | Manager | Get status |
| `/api/v1/sandboxes/{id}/pause` | POST | Manager | Pause sandbox |
| `/api/v1/sandboxes/{id}/resume` | POST | Manager | Resume sandbox |
| `/api/v1/sandboxes/{id}/refresh` | POST | Manager | Refresh TTL |
| **Context/Process Management** ||||
| `/api/v1/sandboxes/{id}/contexts` | POST | Procd | Create context |
| `/api/v1/sandboxes/{id}/contexts` | GET | Procd | List contexts |
| `/api/v1/sandboxes/{id}/contexts/{ctx_id}` | GET | Procd | Get context |
| `/api/v1/sandboxes/{id}/contexts/{ctx_id}` | DELETE | Procd | Delete context |
| `/api/v1/sandboxes/{id}/contexts/{ctx_id}/restart` | POST | Procd | Restart context |
| `/api/v1/sandboxes/{id}/contexts/{ctx_id}/execute` | POST | Procd | Execute code |
| `/api/v1/sandboxes/{id}/contexts/{ctx_id}/ws` | WS | Procd | WebSocket |
| **File System** ||||
| `/api/v1/sandboxes/{id}/files/*` | GET | Procd | Read file/stat/list |
| `/api/v1/sandboxes/{id}/files/*` | POST | Procd | Write file/mkdir |
| `/api/v1/sandboxes/{id}/files/*` | DELETE | Procd | Delete file |
| `/api/v1/sandboxes/{id}/files/watch` | WS | Procd | Watch changes |
| **Template Management** ||||
| `/api/v1/templates` | GET | Manager | List templates |
| `/api/v1/templates/{id}` | GET | Manager | Get template |
| `/api/v1/templates` | POST | Manager | Create template |
| `/api/v1/templates/{id}` | PUT | Manager | Update template |
| `/api/v1/templates/{id}` | DELETE | Manager | Delete template |
| `/api/v1/templates/{id}/pool/warm` | POST | Manager | Warm pool |
| **SandboxVolume Management** ||||
| `/api/v1/sandboxvolumes` | POST | Storage Proxy | Create volume |
| `/api/v1/sandboxvolumes` | GET | Storage Proxy | List volumes |
| `/api/v1/sandboxvolumes/{id}` | GET | Storage Proxy | Get volume |
| `/api/v1/sandboxvolumes/{id}` | DELETE | Storage Proxy | Delete volume |
| `/api/v1/sandboxvolumes/{id}/attach` | POST | **Coordinated** | Attach to sandbox |
| `/api/v1/sandboxvolumes/{id}/detach` | POST | **Coordinated** | Detach from sandbox |
| `/api/v1/sandboxvolumes/{id}/snapshot` | POST | Storage Proxy | Create snapshot |
| `/api/v1/sandboxvolumes/{id}/restore` | POST | Storage Proxy | Restore snapshot |

---

## Authentication

### API Key Format

```
Authorization: Bearer sb0_<team_id>_<random_secret>

Examples:
- sb0_team123_abc123def456789
- sb0_team456_xyz789ghi012345
```

### JWT Format (Optional)

```
Authorization: Bearer <jwt_token>

JWT Claims:
{
  "team_id": "team-123",
  "user_id": "user-456",
  "roles": ["developer"],
  "exp": 1706745600,
  "iat": 1706659200
}
```

### AuthContext

```go
type AuthContext struct {
    AuthMethod  AuthMethod // "api_key", "jwt", "internal"
    TeamID      string
    UserID      string     // JWT only
    APIKeyID    string     // API Key only
    Roles       []string
    Permissions []string
}
```

### Predefined Roles and Permissions

| Role | Permissions |
|------|-------------|
| `admin` | `*:*` (all permissions) |
| `developer` | `sandbox:*`, `template:read`, `sandboxvolume:*` |
| `viewer` | `sandbox:read`, `template:read`, `sandboxvolume:read` |

---

## SandboxVolume Attach/Detach Coordination

Internal Gateway coordinates between Storage Proxy and Procd for volume operations.

### Attach Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway Attach Coordination                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Client Request                                                           │
│     POST /api/v1/sandboxvolumes/{id}/attach                                 │
│     { "sandbox_id": "sb-123", "mount_point": "/workspace" }                  │
│                                                                              │
│  2. Internal Gateway → Storage Proxy (prepare mount)                        │
│     POST http://storage-proxy:8081/api/v1/sandboxvolumes/{id}/attach        │
│     Response: { "token": "eyJ...", "storage_proxy_address": "..." }          │
│                                                                              │
│  3. Internal Gateway → Procd (mount with token)                             │
│     POST http://procd-{sandbox-id}:8080/api/v1/sandboxvolumes/mount          │
│     {                                                                       │
│       "sandboxvolume_id": "sbv-456",                                        │
│       "sandbox_id": "sb-123",                                               │
│       "mount_point": "/workspace",                                          │
│       "token": "eyJ...",                                                     │
│       "storage_proxy_address": "storage-proxy:8080"                         │
│     }                                                                       │
│                                                                              │
│  4. If Procd mount fails → Rollback                                         │
│     POST http://storage-proxy:8081/api/v1/sandboxvolumes/{id}/detach        │
│                                                                              │
│  5. Return to Client                                                         │
│     Response: 200 OK or error                                                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Detach Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway Detach Coordination                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Client Request                                                           │
│     POST /api/v1/sandboxvolumes/{id}/detach                                 │
│     { "sandbox_id": "sb-123" }                                               │
│                                                                              │
│  2. Internal Gateway → Procd (unmount first)                                │
│     POST http://procd-{sandbox-id}:8080/api/v1/sandboxvolumes/unmount        │
│     (Continue even if fails - sandbox may be terminated)                    │
│                                                                              │
│  3. Internal Gateway → Storage Proxy (detach record)                        │
│     POST http://storage-proxy:8081/api/v1/sandboxvolumes/{id}/detach        │
│                                                                              │
│  4. Return to Client                                                         │
│     Response: 200 OK                                                         │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Rate Limiting

### Per-Team Token Bucket

- Default: 100 RPS, 200 burst
- In-memory token bucket with sync.Map
- No external dependencies (Redis not required)

### Response Headers

```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
Retry-After: 1
```

---

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `GATEWAY_HTTP_PORT` | HTTP server port | 8443 |
| `GATEWAY_LOG_LEVEL` | Log level | info |
| `DATABASE_URL` | PostgreSQL connection string | required |
| `MANAGER_URL` | Manager service URL | http://manager:8080 |
| `STORAGE_PROXY_URL` | Storage Proxy URL | http://storage-proxy:8081 |
| `JWT_SECRET` | JWT signing secret | optional |
| `RATE_LIMIT_RPS` | Requests per second per team | 100 |
| `RATE_LIMIT_BURST` | Burst size per team | 200 |
| `ENABLE_METRICS` | Enable Prometheus metrics | true |
| `ENABLE_AUDIT` | Enable audit logging | true |

---

## Database Schema

```sql
-- Teams table
CREATE TABLE teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    quota JSONB NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users table
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    external_id TEXT,
    provider TEXT,
    email TEXT NOT NULL,
    name TEXT,
    primary_team_id TEXT REFERENCES teams(id),
    roles JSONB NOT NULL DEFAULT '[]',
    permissions JSONB NOT NULL DEFAULT '[]',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- API Keys table
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    key_value TEXT NOT NULL UNIQUE,
    team_id TEXT NOT NULL REFERENCES teams(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    type TEXT NOT NULL, -- 'user', 'service', 'internal'
    roles JSONB NOT NULL DEFAULT '[]',
    is_active BOOLEAN NOT NULL DEFAULT true,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    usage_count BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sandboxes table (for routing)
CREATE TABLE sandboxes (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL,
    team_id TEXT NOT NULL REFERENCES teams(id),
    procd_address TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit Logs table
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    team_id TEXT NOT NULL,
    user_id TEXT,
    api_key_id TEXT,
    request_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    latency_ms INTEGER,
    user_agent TEXT,
    client_ip TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Rate Limits table
CREATE TABLE rate_limits (
    team_id TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (team_id, window_start)
);
```

---

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `gateway_http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `gateway_http_request_duration_seconds` | Histogram | method, path | Request latency |
| `gateway_proxy_requests_total` | Counter | target, status | Proxied requests |
| `gateway_auth_failures_total` | Counter | reason | Authentication failures |
| `gateway_rate_limit_hits_total` | Counter | team_id | Rate limit hits |
| `gateway_sandboxvolume_operations_total` | Counter | operation, status | SandboxVolume operations |

---

## Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `/healthz` | Liveness probe |
| `/readyz` | Readiness probe (includes database check) |
| `/metrics` | Prometheus metrics |

---

## Design Advantages

1. **Unified Entry Point**: All external requests go through gateway
2. **Simple Authentication**: API Key primary, JWT optional for SSO
3. **Clear Routing**: Manager for sandbox/template, Storage Proxy for volumes, Procd for process/file
4. **Coordinated Operations**: SandboxVolume attach/detach with rollback support
5. **Full Observability**: Metrics, Tracing, Audit Logging
6. **Low Dependencies**: Only PostgreSQL, no Redis required
7. **High Availability**: Stateless design, horizontal scaling, PodDisruptionBudget
8. **E2B Compatible**: All E2B features have corresponding implementations
