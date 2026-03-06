# Sandbox0 Infra

Sandbox0 Infra is the core infrastructure layer behind Sandbox0: a Kubernetes-native sandbox platform built for low-latency sandbox provisioning, persistent storage, and region-aware self-hosted deployments.

Sandbox0 is released under the Apache 2.0 license. Some enterprise capabilities are protected by built-in license-based feature gates for operators who need them in production environments.

## Highlights

- Sub-200ms class sandbox startup with pre-warmed idle pools managed per template.
- Persistent sandbox volumes with fast snapshot, restore, reuse, and copy workflows.
- Region-scoped control plane and data plane architecture for predictable latency and isolation.
- Kubernetes-native operation with CRDs, operators, ReplicaSets, and DaemonSets.
- Self-hosted friendly design for teams that need private deployment, data locality, and infrastructure control.
- Unified HTTP/WebSocket APIs plus SDKs and CLI for application integration.

## Architecture

Sandbox0 Infra is split across control plane and data plane services:

- `edge-gateway` and `scheduler` run in the control plane.
- `internal-gateway`, `manager`, `storage-proxy`, and `netd` run in the data plane.
- Each provider/region can run as an isolated deployment unit with its own PostgreSQL and S3-compatible storage.

This repository also contains:

- the public API contract in `pkg/apispec/openapi.yaml`
- the infrastructure operator in `infra-operator`
- the dashboard and website applications in `sandbox0-ui`

## Claim A Sandbox

All examples below assume:

- `SANDBOX0_TOKEN` contains a valid API token
- `SANDBOX0_BASE_URL` optionally overrides the default endpoint for self-hosted deployments
- the `default` template already exists

### Go

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    sandbox0 "github.com/sandbox0-ai/sdk-go"
)

func main() {
    client, err := sandbox0.NewClient(
        sandbox0.WithToken(os.Getenv("SANDBOX0_TOKEN")),
    )
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    sandbox, err := client.ClaimSandbox(ctx, "default",
        sandbox0.WithSandboxTTL(300),
        sandbox0.WithSandboxHardTTL(3600),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Sandbox ID: %s\n", sandbox.ID)
    fmt.Printf("Status: %s\n", sandbox.Status)

    defer client.DeleteSandbox(ctx, sandbox.ID)
}
```

### Python

```python
import os

from sandbox0 import Client
from sandbox0.apispec.models.sandbox_config import SandboxConfig

client = Client(
    token=os.environ["SANDBOX0_TOKEN"],
)

with client.sandboxes.open(
    "default",
    config=SandboxConfig(ttl=300, hard_ttl=3600),
) as sandbox:
    print(f"Sandbox ID: {sandbox.id}")
    print(f"Status: {sandbox.status}")
```

### TypeScript

```typescript
import { Client } from "sandbox0";

const client = new Client({
    token: process.env.SANDBOX0_TOKEN!,
    baseUrl: process.env.SANDBOX0_BASE_URL,
});

async function main() {
    const sandbox = await client.sandboxes.claim("default", {
        ttl: 300,
        hardTtl: 3600,
    });

    try {
        console.log("Sandbox ID:", sandbox.id);
        console.log("Status:", sandbox.status);
    } finally {
        await client.sandboxes.delete(sandbox.id);
    }
}

main().catch(console.error);
```

### CLI

```bash
# Claim a sandbox from the "default" template
s0 sandbox create --template default --ttl 300 --hard-ttl 3600

# Delete it when finished
s0 sandbox delete <sandbox-id>
```

## Self-Hosted

For deployment and operational guidance, see the self-hosted documentation:

<https://sandbox0.ai/docs/self-hosted>
