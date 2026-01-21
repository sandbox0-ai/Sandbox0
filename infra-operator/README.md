# Sandbox0Infra Operator

Kubernetes Operator for managing sandbox0 infrastructure deployment.

## Overview

The Sandbox0Infra Operator automates the deployment and management of sandbox0 infrastructure components. It supports three deployment modes:

- **all**: Local development mode with all components and builtin database/storage
- **control-plane**: Cloud control plane with edge-gateway and scheduler
- **data-plane**: Cloud data plane with internal-gateway, manager, storage-proxy, and netd

## Features

- One-click deployment with `kubectl apply -f sandbox0infra.yaml`
- Automatic Ed25519 key pair generation for internal authentication
- Unified health status via `.status.conditions`
- Self-healing capabilities for failed services
- Multi-environment support (local dev, cloud control-plane, cloud data-plane)

## Prerequisites

- Kubernetes cluster v1.35+
- kubectl configured to access your cluster
- cert-manager (optional, for webhook certificates)

## Installation

### Install CRDs

```bash
make install
```

### Deploy the Operator

```bash
make deploy IMG=sandbox0ai/operator:latest
```

Or using kustomize:

```bash
kubectl apply -k config/default
```

## Usage

### Local Development Mode (all)

Deploy all components with builtin PostgreSQL and MinIO:

```yaml
apiVersion: infra.sandbox0.ai/v1alpha1
kind: Sandbox0Infra
metadata:
  name: sandbox0-dev
  namespace: default
spec:
  mode: all
  version: "v0.1.0"
  database:
    type: builtin
  storage:
    type: builtin
  initUser:
    enabled: true
    email: "admin@localhost"
    passwordSecret:
      name: admin-password
```

First, create the admin password secret:

```bash
kubectl create secret generic admin-password --from-literal=password='your-secure-password'
```

Then apply the CR:

```bash
kubectl apply -f config/samples/infra_v1alpha1_sandbox0infra.yaml
```

### Cloud Control Plane Mode

Deploy control plane with external database and storage:

```yaml
apiVersion: infra.sandbox0.ai/v1alpha1
kind: Sandbox0Infra
metadata:
  name: sandbox0-control-plane
  namespace: sandbox0-system
spec:
  mode: control-plane
  version: "v0.1.0"
  database:
    type: postgres
    external:
      host: your-db.rds.amazonaws.com
      database: sandbox0
      username: sandbox0
      passwordSecret:
        name: db-credentials
  storage:
    type: s3
    s3:
      bucket: sandbox0-prod
      region: us-east-1
      credentialsSecret:
        name: aws-credentials
  services:
    edgeGateway:
      replicas: 3
      ingress:
        enabled: true
        className: nginx
        host: api.sandbox0.io
        tlsSecret: api-tls
```

After deployment, get the public key for data plane:

```bash
kubectl get sandbox0infra sandbox0-control-plane -o jsonpath='{.status.internalAuth.controlPlanePublicKey}'
```

### Cloud Data Plane Mode

Deploy data plane connecting to control plane:

```yaml
apiVersion: infra.sandbox0.ai/v1alpha1
kind: Sandbox0Infra
metadata:
  name: sandbox0-data-plane
  namespace: sandbox0-system
spec:
  mode: data-plane
  version: "v0.1.0"
  database:
    type: postgres
    external:
      host: your-db.rds.amazonaws.com
      database: sandbox0
      username: sandbox0
      passwordSecret:
        name: db-credentials
  storage:
    type: s3
    s3:
      bucket: sandbox0-prod
      region: us-east-1
      credentialsSecret:
        name: aws-credentials
  controlPlane:
    url: https://api.sandbox0.io
    internalAuthPublicKeySecret:
      name: control-plane-public-key
  cluster:
    id: cluster-001
    name: Production US East 1
```

First, copy the control plane public key to data plane cluster:

```bash
# On control plane cluster
kubectl get secret sandbox0-control-plane-sandbox0-internal-jwt-control-plane \
  -o jsonpath='{.data.public\.key}' | base64 -d > control-plane.public.key

# On data plane cluster
kubectl create secret generic control-plane-public-key \
  --from-file=public.key=control-plane.public.key
```

## Status and Conditions

Check the deployment status:

```bash
kubectl get sandbox0infra -o wide
```

View detailed conditions:

```bash
kubectl get sandbox0infra sandbox0-dev -o jsonpath='{.status.conditions}' | jq
```

Available condition types:
- `Ready`: Overall readiness
- `DatabaseReady`: Database connectivity
- `StorageReady`: Storage accessibility
- `EdgeGatewayReady`: Edge gateway health
- `InternalGatewayReady`: Internal gateway health
- `ManagerReady`: Manager service health
- `StorageProxyReady`: Storage proxy health
- `NetdReady`: Netd daemon health
- `SchedulerReady`: Scheduler health
- `InternalAuthReady`: Key generation status

## Development

### Build

```bash
make build
```

### Run locally

```bash
make run
```

### Run tests

```bash
make test
```

### Build and push image

```bash
make docker-build docker-push IMG=sandbox0ai/operator:latest
```

## Uninstall

```bash
# Remove CRs
kubectl delete sandbox0infras --all

# Remove CRDs
make uninstall

# Remove operator
make undeploy
```


