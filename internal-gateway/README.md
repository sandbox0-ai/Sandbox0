# Internal Gateway

Internal Gateway is the unified entry point for sandbox0, responsible for authentication, routing, rate limiting, and service coordination.

## Features

- **Authentication**: API Key and JWT authentication support
- **Authorization**: Role-based access control (RBAC)
- **Rate Limiting**: Per-team rate limiting with token bucket algorithm
- **Request Routing**: Routes requests to Manager, Procd, or Storage Proxy
- **SandboxVolume Coordination**: Orchestrates attach/detach operations between Storage Proxy and Procd
- **Audit Logging**: Full request audit trail to PostgreSQL
- **Metrics**: Prometheus metrics for monitoring
- **Health Checks**: Kubernetes-compatible health endpoints

## Architecture

```
                                 ┌─────────────────┐
                                 │ Internal Gateway│
                                 │    (Port 8443)  │
                                 └────────┬────────┘
                                          │
        ┌─────────────────────────────────┼─────────────────────────────────┐
        │                                 │                                 │
        ▼                                 ▼                                 ▼
┌───────────────┐               ┌───────────────┐               ┌───────────────┐
│    Manager    │               │     Procd     │               │ Storage Proxy │
│  (Port 8080)  │               │  (Dynamic)    │               │  (Port 8081)  │
└───────────────┘               └───────────────┘               └───────────────┘
```

## API Routes

### Sandbox Management (→ Manager)
- `POST /api/v1/sandboxes` - Create sandbox
- `GET /api/v1/sandboxes` - List sandboxes
- `GET /api/v1/sandboxes/{id}` - Get sandbox
- `DELETE /api/v1/sandboxes/{id}` - Delete sandbox

### Process/Context Management (→ Procd)
- `POST /api/v1/sandboxes/{id}/contexts` - Create context
- `POST /api/v1/sandboxes/{id}/contexts/{ctx_id}/execute` - Execute code
- `WS /api/v1/sandboxes/{id}/contexts/{ctx_id}/ws` - WebSocket connection

### File System (→ Procd)
- `GET /api/v1/sandboxes/{id}/files/*` - Read file/directory
- `POST /api/v1/sandboxes/{id}/files/*` - Write file/create directory
- `DELETE /api/v1/sandboxes/{id}/files/*` - Delete file/directory

### Template Management (→ Manager)
- `GET /api/v1/templates` - List templates
- `POST /api/v1/templates` - Create template

### SandboxVolume Management (→ Storage Proxy + Procd)
- `POST /api/v1/sandboxvolumes` - Create volume
- `POST /api/v1/sandboxvolumes/{id}/attach` - Attach to sandbox (coordinated)
- `POST /api/v1/sandboxvolumes/{id}/detach` - Detach from sandbox (coordinated)
- `POST /api/v1/sandboxvolumes/{id}/snapshot` - Create snapshot
- `POST /api/v1/sandboxvolumes/{id}/restore` - Restore from snapshot

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `GATEWAY_HTTP_PORT` | HTTP server port | 8443 |
| `GATEWAY_LOG_LEVEL` | Log level (debug/info/warn/error) | info |
| `DATABASE_URL` | PostgreSQL connection string | required |
| `MANAGER_URL` | Manager service URL | http://manager:8080 |
| `STORAGE_PROXY_URL` | Storage Proxy service URL | http://storage-proxy:8081 |
| `JWT_SECRET` | JWT signing secret | required for JWT auth |
| `RATE_LIMIT_RPS` | Requests per second per team | 100 |
| `RATE_LIMIT_BURST` | Burst size per team | 200 |
| `ENABLE_METRICS` | Enable Prometheus metrics | true |
| `ENABLE_AUDIT` | Enable audit logging | true |

## Building

```bash
# Build binary
make build

# Run locally
make run-local

# Build Docker image
make docker-build
```

## Deployment

```bash
# Apply Kubernetes manifests
kubectl apply -k deploy/k8s/

# Or apply individual files
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/secret.yaml
kubectl apply -f deploy/k8s/networkpolicy.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

## Authentication

### API Key
```
Authorization: Bearer sb0_<team_id>_<random_secret>
```

### JWT (Optional)
```
Authorization: Bearer <jwt_token>
```

## Metrics

Prometheus metrics are available at `/metrics`:
- `gateway_http_requests_total` - Total HTTP requests
- `gateway_http_request_duration_seconds` - Request latency
- `gateway_proxy_requests_total` - Proxied requests
- `gateway_auth_failures_total` - Authentication failures
- `gateway_rate_limit_hits_total` - Rate limit hits

## Health Checks

- `/healthz` - Liveness probe
- `/readyz` - Readiness probe (includes database connectivity check)

