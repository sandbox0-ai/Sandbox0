# Procd 设计规范

## 一、设计目标

Procd 是 Sandbox0 的核心容器组件（PID=1），负责沙箱内的资源管理和进程控制。

### 核心职责

1. **进程管理**：统一的进程抽象，支持 REPL 和 Shell 两种进程类型
2. **Volume 管理**：持久化存储的挂载、快照、恢复
3. **网络隔离**：动态网络策略，IP/CIDR 过滤、域名过滤、DNS 欺骗防护
4. **文件操作**：文件读写、目录监听、文件事件推送

---

## 二、架构概览

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Procd Architecture                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Procd (PID=1)                                                              │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                        HTTP Server (Port: 8080)                       │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│   │  │  Process    │ │   Volume    │ │   Network   │ │   Health    │     │  │
│   │  │   APIs      │ │    APIs     │ │    APIs     │ │    Check    │     │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                        Core Managers                                   │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐     │  │
│   │  │  Context    │ │  Volume     │ │  Network    │ │   File      │     │  │
│   │  │  Manager    │ │  Manager    │ │  Manager    │ │  Manager    │     │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘     │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                         │
│                                    ▼                                         │
│   ┌───────────────────────────────────────────────────────────────────────┐  │
│   │                      Subprocess Layer                                  │  │
│   │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                     │  │
│   │  │ REPL Proc   │ │ Shell Proc  │ │  Commands   │                     │  │
│   │  │ (IPython)   │ │  (Bash)     │ │  (Exec)     │                     │  │
│   │  └─────────────┘ └─────────────┘ └─────────────┘                     │  │
│   └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 子模块索引

| 子模块 | 说明 | 文档 |
|--------|------|------|
| **Process** | 进程管理（REPL、Shell） | [process.md](./process.md) |
| **Volume** | 持久化存储管理 | [volume.md](./volume.md) |
| **Network** | 网络隔离与策略控制 | [network.md](./network.md) |
| **File** | 文件系统操作与监听 | [file.md](./file.md) |

---

## 三、HTTP API（对外契约）

### 3.1 Context/进程管理

```http
# 创建 Context
POST /api/v1/contexts
Content-Type: application/json

{
    "type": "repl",           # "repl" | "shell"
    "language": "python",     # python | node | ruby | r | bash | zsh
    "cwd": "/home/user",
    "env_vars": {"API_KEY": "xxx"}
}

Response: 201 Created
{
    "id": "ctx-abc123",
    "type": "repl",
    "language": "python",
    "cwd": "/home/user",
    "main_process": {
        "id": "proc-123",
        "pid": 1234,
        "type": "repl"
    },
    "created_at": "2024-01-01T00:00:00Z"
}

# 列出 Context
GET /api/v1/contexts

# 获取 Context
GET /api/v1/contexts/{id}

# 删除 Context
DELETE /api/v1/contexts/{id}

# 重启 Context
POST /api/v1/contexts/{id}/restart

# 执行代码 (REPL Context)
POST /api/v1/contexts/{id}/execute
Content-Type: application/json

{
    "code": "x = 100\nprint(x)"
}

Response: 200 OK (流式 SSE)
Content-Type: text/event-stream

data: {"type":"pty","text":"100\n"}
data: {"type":"end"}

# 执行命令 (Shell Context)
POST /api/v1/contexts/{id}/command
Content-Type: application/json

{
    "command": "npm install express"
}

# WebSocket 连接
WS /api/v1/contexts/{id}/ws
```

### 3.2 Volume 管理

```http
# 挂载 Volume
POST /api/v1/volumes/{volume_id}/mount
Content-Type: application/json

{
    "sandbox_id": "sb-abc123",
    "mount_point": "/workspace",
    "read_only": false,
    "snapshot_id": "snap-001",
    "warmup_config": {
        "enabled": true,
        "base_layer_ids": ["base-python-3.11"]
    }
}

Response: 200 OK
{
    "volume_id": "vol-123",
    "mount_point": "/workspace",
    "layer_chain": ["base-python-3.11", "delta-pip-pkgs", "working-xyz"],
    "is_from_cache": true
}

# 卸载 Volume
POST /api/v1/volumes/{volume_id}/unmount

# 获取 Volume 状态
GET /api/v1/volumes/{volume_id}

# 创建快照
POST /api/v1/volumes/{volume_id}/snapshots

# 列出快照
GET /api/v1/volumes/{volume_id}/snapshots

# 恢复快照
POST /api/v1/volumes/{volume_id}/restore

# 预加载 Layer
POST /api/v1/volumes/preload
```

### 3.3 网络策略

```http
# 获取当前网络策略
GET /api/v1/network/policy

Response: 200 OK
{
    "mode": "whitelist",
    "egress": {
        "allow_cidrs": ["8.8.8.8", "1.1.1.0/24"],
        "allow_domains": ["google.com", "*.github.com"],
        "deny_cidrs": ["10.0.0.0/8"],
        "tcp_proxy_port": 1080
    },
    "updated_at": "2024-01-01T00:00:00Z"
}

# 更新网络策略
PUT /api/v1/network/policy

# 重置为默认策略
POST /api/v1/network/policy/reset
```

### 3.4 文件操作

```http
# 读文件
GET /api/v1/files/*path

Response: 200 OK
{
    "content": "file content base64 encoded",
    "size": 1024,
    "mode": "0644",
    "mod_time": "2024-01-01T00:00:00Z"
}

# 写文件
POST /api/v1/files/*path
Content-Type: application/json

{
    "content": "base64 encoded content",
    "mode": "0644"
}

# 获取文件/目录信息 (Stat)
GET /api/v1/files/*path?stat=true

Response: 200 OK
{
    "name": "test.txt",
    "path": "/workspace/test.txt",
    "type": "file",  # "file" | "dir" | "symlink"
    "size": 1024,
    "mode": "0644",
    "mod_time": "2024-01-01T00:00:00Z",
    "is_link": false,
    "link_target": ""
}

# 创建目录 (MakeDir)
POST /api/v1/files/*path?mkdir=true
Content-Type: application/json

{
    "mode": "0755",
    "recursive": false  # 是否递归创建父目录
}

# 重命名/移动文件 (Move)
POST /api/v1/files/move
Content-Type: application/json

{
    "src": "/workspace/old.txt",
    "dst": "/workspace/new.txt"
}

# 列出目录
GET /api/v1/files/*path?list=true

Response: 200 OK
{
    "path": "/workspace",
    "entries": [
        {
            "name": "test.txt",
            "type": "file",
            "size": 1024,
            "mode": "0644",
            "mod_time": "2024-01-01T00:00:00Z"
        },
        {
            "name": "subdir",
            "type": "dir",
            "mode": "0755",
            "mod_time": "2024-01-01T00:00:00Z"
        }
    ]
}

# 删除文件
DELETE /api/v1/files/*path

# 监听文件变化 (WatchDir)
WS /api/v1/files/watch

# WebSocket连接后发送订阅消息
{
    "action": "subscribe",
    "path": "/workspace",
    "recursive": true
}

# 服务器推送文件事件
{
    "type": "create",  # "create" | "write" | "remove" | "rename" | "chmod"
    "path": "/workspace/test.txt",
    "timestamp": "2024-01-01T00:00:00Z"
}

# 取消订阅
{
    "action": "unsubscribe",
    "watch_id": "watch-123"
}
```

### 3.5 健康检查

```http
GET /healthz

Response: 200 OK
{
    "status": "healthy",
    "version": "v1.0.0"
}
```

---

## 四、统一数据结构

### 4.1 Process 接口（所有进程类型都实现）

```go
// Process 统一进程接口
type Process interface {
    // 基本信息
    ID() string                    // 进程唯一标识
    Type() ProcessType             // 进程类型
    PID() int                      // 系统进程ID

    // 生命周期管理
    Start() error                  // 启动进程
    Stop() error                   // 停止进程
    Restart() error                // 重启进程
    IsRunning() bool               // 是否运行中

    // I/O操作
    WriteInput(data []byte) error  // 写入stdin
    ReadOutput() <-chan ProcessOutput  // 读取输出(流式)

    // 状态查询
    ExitCode() (int, error)        // 退出码
    ResourceUsage() ResourceUsage  // 资源使用情况

    // REPL特有方法（通过类型断言访问）
    ExecuteCode(code string) (*ExecutionResult, error)
    GetVariables() map[string]interface{}
    SetVariables(vars map[string]interface{}) error

    // Shell特有方法
    ExecuteCommand(cmd string) (*ExecutionResult, error)
    ResizeTerminal(size PTYSize) error
}

type ProcessType string

const (
    ProcessTypeREPL  ProcessType = "repl"   // REPL进程
    ProcessTypeShell ProcessType = "shell"  // Shell进程
)

type ProcessOutput struct {
    Timestamp time.Time
    Source    OutputSource  // stdout/stderr/pty
    Data      []byte
}
```

### 4.2 Context 结构

```go
// Context 上下文（进程的逻辑容器）
type Context struct {
    ID          string
    Type        ProcessType  // 主进程类型(repl或shell)
    Language    string        // python/node/bash等
    CWD         string
    EnvVars     map[string]string
    MainProcess Process      // 主进程(REPL或Shell)
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

---

## 五、启动配置

### 5.1 环境变量

```bash
# 必需
SANDBOX_ID=sb-abc123              # 沙箱ID
TEMPLATE_ID=python-dev            # 模板ID

# 可选
PROCD_LOG_LEVEL=info              # 日志级别
PROCD_MAX_CONTEXTS=100            # 最大Context数
PROCD_VOLUME_CACHE_SIZE=10Gi      # Volume缓存大小
```

### 5.2 启动流程

```go
func main() {
    // 1. 解析配置
    config := loadConfig()

    // 2. 初始化网络管理器
    networkManager := NewNetworkManager(config.Network)

    // 3. 初始化 VolumeManager
    volumeManager := NewVolumeManager(config.Volume)

    // 4. 初始化 ContextManager
    contextManager := NewContextManager(config.Context)

    // 5. 设置网络
    networkManager.SetupNetwork()

    // 6. 启动 HTTP 服务器
    server := NewProcdServer(networkManager, volumeManager, contextManager)
    server.Start(":8080")
}
```

---

## 六、与其他组件的关系

### 6.1 与 Manager 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                   Manager ↔ Procd 交互                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Manager                                      Procd                         │
│     │                                           │                          │
│     │ 1. 认领空闲Pod                               │                          │
│     │    PUT /api/v1/network/policy              │                          │
│     ├──────────────────────────────────────────► │                          │
│     │                                           │ 2. 应用网络策略             │
│     │                                           │                          │
│     │ 3. 挂载Volume                               │                          │
│     │    POST /api/v1/volumes/{id}/mount        │                          │
│     ├──────────────────────────────────────────► │                          │
│     │                                           │ 4. 创建OverlayFS           │
│     │                                           │                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 与 Internal Gateway 的交互

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                 Internal Gateway → Procd                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Gateway                                    Procd                           │
│     │                                        │                              │
│     │ 1. 创建Context                           │                              │
│     │    POST /api/v1/sandboxes/{id}/contexts │                              │
│     ├───────────────────────────────────────► │                              │
│     │                                        │ 2. 启动REPL/Shell进程        │
│     │                                        │                              │
│     │ 3. 执行代码                              │                              │
│     │    POST /api/v1/contexts/{id}/execute  │                              │
│     ├───────────────────────────────────────► │                              │
│     │                                        │ 4. 写入PTY并读取输出         │
│     │                                        │                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、技术选型

| 组件 | 技术选择 | 说明 |
|------|----------|------|
| HTTP Server | Go stdlib | 无需额外依赖 |
| PTY | github.com/creack/pty | 伪终端支持 |
| OverlayFS | kernel syscall | 内核级 CoW |
| nftables | google.com/googleapis/... | 网络防火墙 |
| S3 | AWS SDK | 持久化存储 |

---

## 八、限制与约束

1. **单进程模式**：Procd 作为 PID=1 运行，不使用多进程
2. **内存限制**：所有 Context 共享容器内存配额
3. **网络隔离**：每个沙箱独立的网络命名空间
4. **存储路径**：Volume 挂载点固定在 `/workspace` 或用户指定路径

---

## 九、监控指标

```
# 进程指标
procd_contexts_total              # 当前Context总数
procd_contexts_by_type            # 按类型统计
procd_process_start_duration_ms   # 进程启动耗时

# Volume指标
procd_volumes_mounted             # 已挂载数量
procd_volume_mount_duration_ms    # 挂载耗时
procd_volume_cache_hit_rate       # 缓存命中率

# 网络指标
procd_network_rules_total         # 防火墙规则数
procd_network_policy_updates      # 策略更新次数
```

---

## 十、TDD 开发计划

### 10.1 已有基础

```
pkg/api/procd/                    # 契约接口定义 (已完成)
├── client.go                     # ProcdClient 接口
├── types.go                      # 数据类型定义
├── client_mock.go                # Mock 实现
└── client_contract_test.go       # 契约测试

pkg/test/                         # 测试工具 (已完成)
├── httputil/server.go            # HTTP 测试服务器
└── fixtures/fixtures.go          # 测试数据 fixture
```

### 10.2 开发阶段

```
Phase 0: 项目初始化
├── 0.1 创建 procd 项目结构
├── 0.2 配置依赖管理
└── 0.3 创建基础测试框架

Phase 1: 核心层 - 子进程抽象 (最先实现，被其他模块依赖)
├── 1.1 Process 接口定义
│   └── internal/process/interface.go
├── 1.2 REPL 进程实现
│   └── internal/process/repl.go
└── 1.3 Shell 进程实现
    └── internal/process/shell.go

Phase 2: 管理层 - 各模块 Manager (可并行开发)
├── 2.1 ContextManager (进程编排)
│   └── internal/context/manager.go
├── 2.2 VolumeManager (存储管理)
│   └── internal/volume/manager.go
├── 2.3 NetworkManager (网络隔离)
│   └── internal/network/manager.go
└── 2.4 FileManager (文件操作)
    └── internal/file/manager.go

Phase 3: HTTP API 层 - 对外接口
├── 3.1 Context/Process APIs
│   └── internal/api/context_handler.go
├── 3.2 Volume APIs
│   └── internal/api/volume_handler.go
├── 3.3 Network APIs
│   └── internal/api/network_handler.go
├── 3.4 File APIs
│   └── internal/api/file_handler.go
└── 3.5 WebSocket 流式接口
    └── internal/api/websocket.go

Phase 4: 集成与 E2E 测试
├── 4.1 模块集成测试
└── 4.2 契约测试验证
```

### 10.3 目录结构

```
infra/procd/
├── cmd/
│   └── procd/
│       └── main.go              # 主程序入口
├── internal/
│   ├── process/                 # 进程抽象层
│   │   ├── interface.go         # Process 接口定义
│   │   ├── repl.go              # REPL 进程实现
│   │   ├── shell.go             # Shell 进程实现
│   │   └── pty.go               # PTY 操作封装
│   ├── context/                 # Context 管理
│   │   ├── manager.go           # ContextManager
│   │   └── manager_test.go
│   ├── volume/                  # Volume 管理
│   │   ├── manager.go           # VolumeManager
│   │   ├── overlayfs.go         # OverlayFS 操作
│   │   └── manager_test.go
│   ├── network/                 # 网络管理
│   │   ├── manager.go           # NetworkManager
│   │   ├── nftables.go          # nftables 操作
│   │   └── manager_test.go
│   ├── file/                    # 文件管理
│   │   ├── manager.go           # FileManager
│   │   ├── watcher.go           # fsnotify 封装
│   │   └── manager_test.go
│   ├── api/                     # HTTP handlers
│   │   ├── server.go            # HTTP Server
│   │   ├── context_handler.go   # Context APIs
│   │   ├── volume_handler.go    # Volume APIs
│   │   ├── network_handler.go   # Network APIs
│   │   ├── file_handler.go      # File APIs
│   │   ├── websocket.go         # WebSocket handlers
│   │   └── middleware.go        # 中间件
│   └── testutil/                # 内部测试工具
│       ├── testserver.go        # 测试 HTTP 服务器
│       └── process_mock.go      # Process 接口 mock
├── go.mod
├── go.sum
├── Makefile
├── procd_test.go                # E2E 测试
└── README.md
```

### 10.4 TDD 工作流示例

```go
// Step 1: 写测试 (Red)
func TestREPLProcess_ExecuteCode(t *testing.T) {
    // Given
    repl := NewREPLProcess("python")
    err := repl.Start()
    require.NoError(t, err)
    defer repl.Stop()

    // When
    result, err := repl.ExecuteCode("x = 42\nprint(x)")

    // Then
    require.NoError(t, err)
    assert.Contains(t, result.Output, "42")
}

// Step 2: 运行测试 -> 失败
// go test ./internal/process -v
// FAIL: TestREPLProcess_ExecuteCode: not implemented yet

// Step 3: 实现最少代码使测试通过 (Green)
func (r *REPLProcess) ExecuteCode(code string) (*ExecutionResult, error) {
    r.pty.Write([]byte(code + "\n"))
    var output []byte
    buf := make([]byte, 1024)
    n, _ := r.pty.Read(buf)
    output = append(output, buf[:n]...)
    return &ExecutionResult{Output: string(output)}, nil
}

// Step 4: 重构优化
// 提取公共逻辑、增加错误处理、添加更多测试

// Step 5: 重复循环
```

### 10.5 测试优先级

| 优先级 | 测试类型 | 说明 |
|--------|----------|------|
| P0 | Process 单元测试 | 核心进程抽象，必须最先实现 |
| P0 | Manager 单元测试 | 使用 Process mock，可并行开发 |
| P1 | Handler 契约测试 | 验证 API 符合 spec 定义 |
| P1 | Handler 单元测试 | 使用 Manager mock |
| P2 | 集成测试 | 真实组件集成 |
| P3 | E2E 测试 | 完整流程测试 |

### 10.6 依赖关系

```
Phase 1 (Process) ──────┐
                         ├──> Phase 3 (API)
Phase 2 (Managers) ─────┘
                         │
                         └──> Phase 4 (E2E)
```

### 10.7 关键注意事项

1. **Spec 即文档**: 每个功能实现前先阅读对应的 spec 文件
2. **依赖顺序**: Phase 1 必须先完成，Phase 2 各模块可并行
3. **Mock 优先**: Manager 层测试使用 Process mock
4. **契约测试**: API 层必须通过 `pkg/api/procd/client_contract_test.go`
5. **特权操作**: Volume/Network 测试需要容器特权模式
6. **无中文**: 代码和注释不能包含中文
