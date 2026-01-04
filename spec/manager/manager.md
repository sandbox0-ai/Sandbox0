# Sandbox0 Manager 设计规范

## 一、设计目标

Manager 是 sandbox0 的核心管理服务，负责：
1. **沙箱模板管理**：通过 K8s Operator 维护 `SandboxTemplate` CRD
2. **沙箱实例认领**：提供 HTTP API 接收 internal-gateway 请求来实例化沙箱
3. **资源调度**：根据模板配置和资源水位进行沙箱调度
4. **持久卷管理**：管理 Volume 元数据（存储在 PostgreSQL），协调 Procd 挂载

---

## 二、架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Manager Architecture                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                        HTTP Server (Port: 8080)                       │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │   Claim     │ │    List     │ │   Status    │ │  Terminate  │     │  │
│  │  │  Sandbox    │ │  Sandboxes  │ │   Query     │ │   Sandbox   │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                           Service Layer                                │  │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│  │  │   Sandbox   │ │   Template  │ │   Resource  │ │    Cleanup  │     │  │
│  │  │   Service   │ │   Service   │ │  Monitor    │ │  Controller │     │  │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│              ┌─────────────────────┴─────────────────────┐                 │
│              ▼                                           ▼                 │
│  ┌───────────────────────────────────┐   ┌─────────────────────────────┐  │
│  │     K8s Operator/Informer         │   │      Kubernetes API         │  │
│  │                                   │   │                             │  │
│  │  - Watch SandboxTemplate CRD      │   │  - ReplicaSet (minIdle)    │  │
│  │  - Watch Pods (informer cache)    │   │  - Pods (状态存在           │  │
│  │  - Event Handler                  │   │    annotations)            │  │
│  │                                   │   │  - K8s Events               │  │
│  └───────────────────────────────────┘   └─────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 核心设计：ReplicaSet + Pod 状态存储

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        State Storage in k8s                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. SandboxTemplate CRD - 模板配置（唯一事实来源）                            │
│     ├── spec.pool.minIdle - ReplicaSet 副本数                                │
│     └── status - 活跃/空闲数统计                                             │
│                                                                              │
│  2. ReplicaSet - 维护空闲池 (replicas = minIdle)                              │
│     └── Selector: pool-type=idle                                             │
│                                                                              │
│  3. Pod - Sandbox 实例                                                       │
│     ├── Labels:                                                             │
│     │   ├── sandbox0.ai/template-id: python-dev                              │
│     │   ├── sandbox0.ai/pool-type: idle/active                               │
│     │   └── sandbox0.ai/sandbox-id: sb-xxx                                   │
│     └── Annotations:                                                         │
│         ├── sandbox0.ai/team-id: team-123                                    │
│         ├── sandbox0.ai/user-id: user-456                                    │
│         ├── sandbox0.ai/claimed-at: 2024-01-01T00:00:00Z                     │
│         ├── sandbox0.ai/expires-at: 2024-01-01T01:00:00Z                     │
│         └── sandbox0.ai/config: {"env_vars": {...}, "ttl": 3600}            │
│                                                                              │
│  4. Cleanup Controller                                                       │
│     ├── 监控 maxIdle（List Pods 删除多余）                                   │
│     ├── 过期清理（List Pods 过滤 expires_at < now）                           │
│     └── 回收释放（active → idle 改 labels）                                  │
│                                                                              │
│  5. Volume Storage (Unified PostgreSQL + S3)                                │
│     ├── PostgreSQL - Volume 元数据 + JuiceFS 文件系统元数据（统一存储）        │
│     │   ├── Manager tables: volumes, snapshots, teams, etc.                 │
│     │   └── JuiceFS tables: jfs_node, jfs_edge, jfs_chunk, etc.            │
│     └── S3 - JuiceFS 数据块（实际文件内容）                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、SandboxTemplate CRD 设计

### 3.1 CRD 基本结构

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: sandboxtemplates.sandbox0.ai
spec:
  group: sandbox0.ai
  names:
    kind: SandboxTemplate
    plural: sandboxtemplates
    singular: sandboxtemplate
    shortNames:
      - sbt
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### 3.2 Spec 定义

```go
// SandboxTemplateSpec 定义沙箱模板的期望状态
type SandboxTemplateSpec struct {
    // 描述信息
    Description string            `json:"description,omitempty"`
    DisplayName string            `json:"displayName,omitempty"`
    Tags        []string          `json:"tags,omitempty"`

    // 主容器配置（必需）
    MainContainer ContainerSpec    `json:"mainContainer"`

    // Sidecar 容器配置（可选）
    Sidecars []ContainerSpec       `json:"sidecars,omitempty"`

    // Pod 级别配置
    Pod *PodSpecOverride          `json:"pod,omitempty"`

    // 资源配额
    Resources ResourceQuota        `json:"resources"`

    // 网络策略（模板级别的默认策略）
    // 注意：实际运行时策略由Procd管理，Manager通过API动态更新
    Network *NetworkPolicy        `json:"network,omitempty"`

    // 水池配置
    Pool PoolStrategy             `json:"pool"`

    // 生命周期管理
    Lifecycle *LifecyclePolicy    `json:"lifecycle,omitempty"`

    // 模板继承
    Inherits *string              `json:"inherits,omitempty"`

    // 环境变量（全局，所有容器共享）
    EnvVars map[string]string     `json:"envVars,omitempty"`

    // 访问控制
    Public bool                   `json:"public,omitempty"`
    AllowedTeams []string         `json:"allowedTeams,omitempty"`

    // 别名
    Aliases []string              `json:"aliases,omitempty"`

    // 环境配置
    EnvdVersion string            `json:"envdVersion"`
    RuntimeClassName *string      `json:"runtimeClassName,omitempty"`
}

// ContainerSpec Container 配置
type ContainerSpec struct {
    Image           string            `json:"image"`
    ImagePullPolicy string            `json:"imagePullPolicy,omitempty"`
    Command         []string          `json:"command,omitempty"`
    Args            []string          `json:"args,omitempty"`
    Env             []EnvVar          `json:"env,omitempty"`
    VolumeMounts    []VolumeMount     `json:"volumeMounts,omitempty"`
    Resources       ResourceRequirements `json:"resources,omitempty"`
    SecurityContext *SecurityContext  `json:"securityContext,omitempty"`
}

// ResourceRequirements Resource requirements for containers
type ResourceRequirements struct {
    Limits   map[string]string `json:"limits,omitempty"`   // e.g. {"cpu": "2", "memory": "4Gi", "sandbox0.ai/fuse": "1"}
    Requests map[string]string `json:"requests,omitempty"` // e.g. {"cpu": "1", "memory": "2Gi"}
}

// SecurityContext Security context for containers
type SecurityContext struct {
    Capabilities *Capabilities `json:"capabilities,omitempty"`
    RunAsUser    *int64        `json:"runAsUser,omitempty"`
    RunAsGroup   *int64        `json:"runAsGroup,omitempty"`
}

// Capabilities Linux capabilities
type Capabilities struct {
    Add  []string `json:"add,omitempty"`  // e.g. ["NET_ADMIN"]
    Drop []string `json:"drop,omitempty"`
}

// ResourceQuota Resource quota (per template)
type ResourceQuota struct {
    CPU    string `json:"cpu"`    // e.g. "2"
    Memory string `json:"memory"` // e.g. "4Gi"
    GPU    string `json:"gpu,omitempty"` // e.g. "1"
}

// PoolStrategy Pool strategy
type PoolStrategy struct {
    MinIdle int32 `json:"minIdle"` // Minimum idle pods (ReplicaSet replicas)
    MaxIdle int32 `json:"maxIdle"` // Maximum idle pods (enforced by CleanupController)
}

// NetworkPolicy 网络策略 (模板级别的默认策略)
// 注意：实际运行时策略由Procd管理，Manager通过API动态更新
type NetworkPolicy struct {
    Mode    NetworkPolicyMode  `json:"mode"`
    Egress  *NetworkEgressPolicy `json:"egress,omitempty"`
    Ingress *NetworkIngressPolicy `json:"ingress,omitempty"`
}
```

### 3.3 Example: SandboxTemplate with JuiceFS Support

```yaml
apiVersion: sandbox0.ai/v1alpha1
kind: SandboxTemplate
metadata:
  name: python-dev
  namespace: default
spec:
  description: "Python development environment with JuiceFS volume support"
  displayName: "Python Dev"
  
  mainContainer:
    image: sandbox0/procd:latest
    securityContext:
      capabilities:
        add:
          - NET_ADMIN  # For nftables only, no SYS_ADMIN needed
    env:
      - name: JUICEFS_META_URL
        value: "postgres://sandbox0:password@postgres.sandbox0-system.svc.cluster.local:5432/sandbox0?sslmode=disable"
      - name: JUICEFS_S3_BUCKET
        value: "sandbox0-volumes"
      - name: JUICEFS_S3_REGION
        value: "us-east-1"
      - name: JUICEFS_CACHE_SIZE
        value: "10Gi"
    volumeMounts:
      - name: juicefs-cache
        mountPath: /var/lib/juicefs/cache
    resources:
      limits:
        cpu: "2"
        memory: "4Gi"
        sandbox0.ai/fuse: "1"  # Request /dev/fuse from k8s-plugin
      requests:
        cpu: "1"
        memory: "2Gi"
  
  pod:
    volumes:
      - name: juicefs-cache
        emptyDir:
          sizeLimit: 10Gi
  
  resources:
    cpu: "2"
    memory: "4Gi"
  
  pool:
    minIdle: 3
    maxIdle: 10
  
  runtimeClassName: kata  # Optional: Use Kata Containers for better isolation
```

### 3.4 HTTP API

#### 认领沙箱

```http
POST /api/v1/sandboxes/claim
{
    "template_id": "python-dev",
    "team_id": "team-123",
    "user_id": "user-456",
    "sandbox_id": "sandbox-abc",
    "config": {
        "env_vars": {"API_KEY": "xxx"},
        "ttl": 3600
    }
}

Response: 201 Created
{
    "sandbox_id": "sandbox-abc",
    "template_id": "python-dev",
    "status": "starting",
    "procd_address": "sandbox-abc-pod.default.svc.cluster.local:8080"
}
```

#### Volume 管理

```http
# Create Volume
POST /api/v1/volumes
Content-Type: application/json

{
    "name": "my-workspace",
    "team_id": "team-123",
    "capacity": "10Gi",
    "juicefs": {
        "meta_url": "postgres://postgres:5432/sandbox0?sslmode=disable",
        "s3_bucket": "sandbox0-volumes",
        "s3_prefix": "teams/team-123/volumes/vol-abc123",
        "cache_size": "10Gi"
    }
}

Response: 201 Created
{
    "id": "vol-abc123",
    "name": "my-workspace",
    "team_id": "team-123",
    "juicefs": {
        "meta_url": "postgres://postgres:5432/sandbox0?sslmode=disable",
        "s3_bucket": "sandbox0-volumes",
        "s3_prefix": "teams/team-123/volumes/vol-abc123",
        "cache_size": "10Gi"
    },
    "created_at": "2024-01-01T00:00:00Z"
}

# Get Volume
GET /api/v1/volumes/{volume_id}

# List Volumes
GET /api/v1/volumes?team_id=team-123

# Delete Volume
DELETE /api/v1/volumes/{volume_id}

# Attach Volume to Sandbox (triggers Procd mount)
POST /api/v1/volumes/{volume_id}/attach
Content-Type: application/json

{
    "sandbox_id": "sb-abc123",
    "mount_point": "/workspace",
    "read_only": false,
    "snapshot_id": "snap-001"
}

Response: 200 OK
{
    "volume_id": "vol-abc123",
    "sandbox_id": "sb-abc123",
    "mount_point": "/workspace",
    "procd_address": "sb-abc123-pod:8080"
}
```

---

## 四、Manager Operator 逻辑

### 4.1 Reconcile 流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Operator Reconcile Flow (ReplicaSet)                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Watch SandboxTemplate CRD                                               │
│     │                                                                       │
│     ▼                                                                       │
│  2. 检查变化类型 (Create/Update/Delete)                                      │
│     │                                                                       │
│     ├─► Create: 创建 ReplicaSet (replicas=minIdle)                         │
│     ├─► Update: 更新 ReplicaSet replicas                                   │
│     └─► Delete: 清理 ReplicaSet                                            │
│     │                                                                       │
│     ▼                                                                       │
│  3. ReplicaSet 管理                                                         │
│     │                                                                       │
│     ├─► 创建/更新 ReplicaSet                                                │
│     │   - Selector: sandbox0.ai/template-id=<name>, pool-type=idle          │
│     │   - Replicas: minIdle                                                 │
│     │   - Template: Pod Template（含 procd 容器）                            │
│     │                                                                       │
│     ├─► k8s ReplicaSet 控制器自动维持 Pod 数量                              │
│     │   - Pod 不足时立即创建                                                │
│     │   - Pod 被认领移出后立即补充                                          │
│     │                                                                       │
│     ▼                                                                       │
│  4. 状态同步                                                                │
│     │                                                                       │
│     ├─► 从 informer cache 查询实际 idle/active 数量                           │
│     ├─► 更新 CRD Status                                                    │
│     └─► 更新 Conditions                                                    │
│     │                                                                       │
│     ▼                                                                       │
│  5. 事件记录                                                                │
│     │                                                                       │
│     └─► 发送 K8s Events                                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 ReplicaSet 管理

```go
// PoolManager 水池管理器
type PoolManager struct {
    k8sClient   kubernetes.Interface
    podLister   corev1.PodLister       // informer cache
    recorder    record.EventRecorder
}

// ReconcilePool 调节 ReplicaSet
func (pm *PoolManager) ReconcilePool(ctx context.Context, template *crd.SandboxTemplate) error {
    spec := template.Spec

    // 1. 确保 ReplicaSet 存在且配置正确
    rs, err := pm.getOrCreateReplicaSet(ctx, template)
    if err != nil {
        return err
    }

    // 2. 检查 replicas 是否匹配 minIdle
    if *rs.Spec.Replicas != spec.Pool.MinIdle {
        rs.Spec.Replicas = &spec.Pool.MinIdle
        _, err = pm.k8sClient.AppsV1().ReplicaSets(template.Namespace).Update(ctx, rs, metav1.UpdateOptions{})
        if err != nil {
            return err
        }
    }

    // 3. 从 informer cache 查询实际状态并更新 CRD Status
    return pm.updateTemplateStatus(ctx, template)
}
```

### 4.3 Cleanup Controller

```go
// CleanupController 清理控制器
type CleanupController struct {
    k8sClient   kubernetes.Interface
    podLister   corev1.PodLister      // informer cache
    templateLister crd.SandboxTemplateLister
    recorder    record.EventRecorder
    interval    time.Duration
}

// enforceMaxIdle 强制执行 maxIdle 限制
func (cc *CleanupController) enforceMaxIdle(ctx context.Context, template *crd.SandboxTemplate) {
    maxIdle := template.Spec.Pool.MaxIdle

    // 从 informer cache 获取空闲 Pod
    pods, err := cc.podLister.Pods(template.Namespace).List(labels.SelectorFromSet(map[string]string{
        "sandbox0.ai/template-id": template.Name,
        "sandbox0.ai/pool-type":   "idle",
    }))
    if err != nil {
        return
    }

    idleCount := len(pods)
    if int32(idleCount) <= maxIdle {
        return
    }

    // 删除多余的空闲 Pod（保留最新的）
    excess := idleCount - int(maxIdle)
    sort.Slice(pods, func(i, j int) bool {
        return pods[i].CreationTimestamp.After(pods[j].CreationTimestamp.Time)
    })
    for i := 0; i < excess; i++ {
        err := cc.k8sClient.CoreV1().Pods(template.Namespace).Delete(ctx, pods[i].Name, metav1.DeleteOptions{})
        if err != nil {
            continue
        }
    }
}
```

---

## 五、与 Procd 的交互

### 5.1 ProcdClient 接口

```go
// ProcdClient Procd客户端接口 (用于网络策略和Volume挂载)
type ProcdClient interface {
    // UpdateNetworkPolicy 更新网络策略
    UpdateNetworkPolicy(ctx context.Context, procdAddr string, policy *NetworkPolicy) error

    // ResetNetworkPolicy 重置为默认策略 (allow-all)
    ResetNetworkPolicy(ctx context.Context, procdAddr string) error

    // GetNetworkPolicy 获取当前网络策略
    GetNetworkPolicy(ctx context.Context, procdAddr string) (*NetworkPolicy, error)

    // MountVolume 挂载 Volume
    MountVolume(ctx context.Context, procdAddr string, req *VolumeMountRequest) (*VolumeMountResponse, error)

    // UnmountVolume 卸载 Volume
    UnmountVolume(ctx context.Context, procdAddr string, volumeID string) error
}

// VolumeMountRequest Volume 挂载请求
type VolumeMountRequest struct {
    VolumeID    string            `json:"volume_id"`
    SandboxID   string            `json:"sandbox_id"`
    MountPoint  string            `json:"mount_point"`
    ReadOnly    bool              `json:"read_only,omitempty"`
    SnapshotID  string            `json:"snapshot_id,omitempty"`
    JuiceFSOpts map[string]string `json:"juicefs_opts,omitempty"`
}

// VolumeMountResponse Volume 挂载响应
type VolumeMountResponse struct {
    VolumeID       string `json:"volume_id"`
    MountPoint     string `json:"mount_point"`
    JuiceFSVersion string `json:"juicefs_version"`
}
```

### 5.2 网络策略应用

```go
// applyNetworkPolicy 应用网络策略
func (s *SandboxService) applyNetworkPolicy(ctx context.Context, template *crd.SandboxTemplate, procdAddr string, config *SandboxConfig) error {
    // 1. 确定要应用的策略
    var policy *NetworkPolicy

    // 优先级: Config.Network > Template.Spec.Network (默认 allow-all)
    if config != nil && config.Network != nil {
        policy = config.Network
    } else if template.Spec.Network != nil {
        policy = template.Spec.Network
    } else {
        policy = &NetworkPolicy{
            Mode: NetworkModeAllowAll,
        }
    }

    // 2. 调用 Procd API 更新策略
    if err := s.procdClient.UpdateNetworkPolicy(ctx, procdAddr, policy); err != nil {
        return fmt.Errorf("update network policy via procd: %w", err)
    }

    return nil
}
```

### 5.3 Volume 挂载

```go
// VolumeService Volume 管理服务
type VolumeService struct {
    db          *sql.DB           // PostgreSQL
    procdClient ProcdClient
}

// AttachVolume 将 Volume 挂载到 Sandbox
func (s *VolumeService) AttachVolume(ctx context.Context, req *AttachVolumeRequest) (*AttachVolumeResponse, error) {
    // 1. 从 PostgreSQL 加载 Volume 元数据
    volume, err := s.loadVolume(ctx, req.VolumeID)
    if err != nil {
        return nil, fmt.Errorf("load volume: %w", err)
    }

    // 2. 验证访问权限
    if !s.canAccess(volume, req.TeamID, req.SandboxID) {
        return nil, fmt.Errorf("access denied")
    }

    // 3. 获取 Sandbox 的 Procd 地址
    procdAddr, err := s.getProcdAddress(ctx, req.SandboxID)
    if err != nil {
        return nil, fmt.Errorf("get procd address: %w", err)
    }

    // 4. 构造 Procd 挂载请求
    mountReq := &VolumeMountRequest{
        VolumeID:   req.VolumeID,
        SandboxID:  req.SandboxID,
        MountPoint: req.MountPoint,
        ReadOnly:   req.ReadOnly,
        SnapshotID: req.SnapshotID,
        JuiceFSOpts: map[string]string{
            "cache_size": volume.JuiceFS.CacheSize,
            "prefetch":   strconv.Itoa(volume.JuiceFS.Prefetch),
        },
    }

    // 5. 调用 Procd API 挂载 Volume
    mountResp, err := s.procdClient.MountVolume(ctx, procdAddr, mountReq)
    if err != nil {
        return nil, fmt.Errorf("mount volume via procd: %w", err)
    }

    // 6. 更新 Volume 挂载状态
    if err := s.updateVolumeMountStatus(ctx, req.VolumeID, req.SandboxID, true); err != nil {
        return nil, fmt.Errorf("update volume mount status: %w", err)
    }

    return &AttachVolumeResponse{
        VolumeID:      mountResp.VolumeID,
        SandboxID:     req.SandboxID,
        MountPoint:    mountResp.MountPoint,
        ProcdAddress:  procdAddr,
    }, nil
}

// loadVolume 从 PostgreSQL 加载 Volume 元数据
func (s *VolumeService) loadVolume(ctx context.Context, volumeID string) (*Volume, error) {
    var volume Volume
    query := `
        SELECT id, name, team_id, juicefs_config, capacity, encryption_key_id,
               read_only, allowed_sandbox_ids, tags, metadata,
               created_at, updated_at, last_accessed_at, size_bytes, file_count, mount_count
        FROM volumes
        WHERE id = $1
    `
    err := s.db.QueryRowContext(ctx, query, volumeID).Scan(
        &volume.ID, &volume.Name, &volume.TeamID, &volume.JuiceFS, &volume.Capacity,
        &volume.EncryptionKeyID, &volume.ReadOnly, pq.Array(&volume.AllowedSandboxIDs),
        pq.Array(&volume.Tags), &volume.Metadata, &volume.CreatedAt, &volume.UpdatedAt,
        &volume.LastAccessedAt, &volume.SizeBytes, &volume.FileCount, &volume.MountCount,
    )
    if err != nil {
        return nil, err
    }
    return &volume, nil
}
```

---

## 六、与 E2B 功能对比

| 功能 | E2B | Sandbox0 | 说明 |
|------|-----|----------|------|
| 模板定义 | JSON + Dockerfile | CRD | K8s 原生 |
| 多容器 | 不支持 | ✅ Sidecar | 更灵活 |
| 卷挂载 | 单一 S3 | **JuiceFS (S3-backed POSIX)** | 完整 POSIX 语义 |
| Volume管理 | 内置 | **PostgreSQL (unified metadata) + S3 (data)** | 统一元数据存储 |
| 资源配额 | CPU/Memory/Disk | CPU/Memory/GPU | 支持 GPU |
| 水池管理 | 内置 | **ReplicaSet + Cleanup Controller** | k8s 原生，高可靠 |
| 状态存储 | 内置 | **Pod annotations (informer cache)** | 无额外依赖 |
| 事件系统 | 内置 | **k8s Events** | 原生集成 |
| 冷启动 | 取决于池大小 | **ReplicaSet 实时补充** | 池更可靠 |
| 特权模式 | 需要 | **无需 (k8s-plugin)** | 更安全 |
| 元数据引擎 | 内置 | **PostgreSQL (统一)** | 无需 Redis，简化架构 |

---

## 七、总结

### 设计优势

1. **纯 k8s 原生**：CRD + Operator + ReplicaSet + Informer，无额外依赖
2. **高性能**：informer 本地缓存读取 <1ms，比 PG 网络查询更快
3. **高可靠**：ReplicaSet 控制器实时维持 minIdle，毫秒级检测 Pod 缺失
4. **灵活的多容器**：主容器 + Sidecar，支持复杂架构
5. **职责分离**：Operator 管理 ReplicaSet，Cleanup Controller 处理缩容和回收

### 认领流程

```
空闲池认领（热路径）：
1. 从 informer cache 查询空闲 Pod（本地内存，<1ms）
2. 修改 labels（idle → active）和清除 ownerRef
3. ReplicaSet 自动补充新的空闲 Pod
耗时：几十毫秒

池为空认领（冷启动）：
1. 创建独立 Pod（不属于 ReplicaSet）
耗时：取决于镜像拉取时间
```
