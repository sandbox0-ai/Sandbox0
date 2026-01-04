# Procd - 进程管理设计规范

## 一、设计目标

将所有子进程抽象为统一的 `Process` 接口，所有进程都有状态，支持完整的 Shell 特性。

### 核心思想

像真实 Terminal 一样，所有操作在持久化 Shell 环境中执行，支持 `cd`、`export`、管道等完整 Shell 特性。

---

## 二、进程类型

### 2.1 Process 类型层次

```
┌─────────────────────────────────────────────────────────────────┐
│                    Process 类型层次                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Process (interface) - 所有进程都有状态                          │
│    ├─ REPL Process        代码解释器，保持变量/导入                │
│    │   ├─ Python REPL     IPython                              │
│    │   ├─ JavaScript REPL Node.js REPL                         │
│    │   ├─ TypeScript REPL ts-node REPL                         │
│    │   ├─ Ruby REPL        IRB                                  │
│    │   └─ R REPL           R                                    │
│    │                                                          │
│    └─ Shell Process       有状态Shell，像真实Terminal              │
│        ├─ Bash            /bin/bash (默认)                       │
│        ├─ Zsh             /bin/zsh                              │
│        └─ Fish            /usr/bin/fish                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 进程类型对比

| ProcessType | 有状态 | 交互式 | 终端(PTY) | 典型用途 |
|-------------|--------|--------|-----------|----------|
| `repl` | ✅ | ✅ | ✅ | 代码执行、数据分析 |
| `shell` | ✅ | ✅ | ✅ | 命令执行、系统操作 |

**关键区别**：
- **REPL**: 专注语言特性（变量、函数、类），语法由语言定义
- **Shell**: 专注系统操作（文件、进程、管道），语法由 Shell 定义

### 2.3 为什么不用无状态 Command Process

```
┌─────────────────────────────────────────────────────────────────┐
│                  无状态Command vs 有状态Shell                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ❌ Command Process (无状态)                                    │
│     $ ls /home/user/project                                    │
│     $ cat file.txt      → 失败！因为不在那个目录                │
│     $ export X=1                                                │
│     $ echo $X          → 失败！环境变量没保持                   │
│                                                                 │
│  ✅ Shell Process (有状态)                                      │
│     $ cd /home/user/project                                    │
│     $ ls                                                        │
│     $ cat file.txt      → ✅ 正常工作                           │
│     $ export X=1                                                │
│     $ echo $X          → ✅ 输出: 1                             │
│     $ npm install | grep express   → ✅ 管道工作                 │
│     $ vim .env          → ✅ 交互式编辑                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 三、Context（上下文）

### 3.1 概念

Context 是进程的逻辑容器，提供：
- 统一的工作目录
- 共享的环境变量
- 进程生命周期管理
- 输出流管理

### 3.2 结构

```
┌─────────────────────────────────────────────────────────────────┐
│                      Context                                    │
│  ID: ctx-abc123                                                 │
│  CWD: /home/user/project                                        │
│  EnvVars: {API_KEY: "xxx", NODE_ENV: "dev"}                    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  MainProcess (主进程)                                   │   │
│  │  Type: repl or shell                                   │   │
│  │  PID: 1234                                              │   │
│  │                                                          │   │
│  │  如果是REPL:                                             │   │
│  │    x = 100                                              │   │
│  │    import os                                             │   │
│  │    def foo(): ...                                        │   │
│  │                                                          │   │
│  │  如果是Shell:                                            │   │
│  │    cd /home/user                                        │   │
│  │    export NODE_ENV=dev                                  │   │
│  │    npm run dev &                                         │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  主进程本身具备完整的交互能力，不需要额外的"命令进程"              │
└─────────────────────────────────────────────────────────────────┘
```

---

## 四、接口定义

### 4.1 Process 接口

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

type OutputSource string

const (
    OutputSourceStdout OutputSource = "stdout"
    OutputSourceStderr OutputSource = "stderr"
    OutputSourcePTY    OutputSource = "pty"
)
```

### 4.2 ProcessConfig

```go
// ProcessConfig 进程配置
type ProcessConfig struct {
    // 基本配置
    Type     ProcessType
    Language string            // 语言: python, node, ruby, r, bash, zsh

    // 环境配置
    CWD      string            // 工作目录
    EnvVars  map[string]string // 环境变量

    // 执行配置
    AutoRestart bool           // 自动重启

    // PTY配置（所有进程都支持PTY）
    PTYSize  *PTYSize          // PTY终端大小
    Term     string            // TERM类型，默认 xterm-256color

}

type PTYSize struct {
    Rows uint16
    Cols uint16
}
```

### 4.3 ContextManager 接口

```go
// Context 上下文
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

// ContextManager 上下文管理器
type ContextManager interface {
    // Context管理
    CreateContext(config ProcessConfig) (*Context, error)
    GetContext(id string) (*Context, error)
    ListContexts() []*Context
    DeleteContext(id string) error
    RestartContext(id string) (*Context, error)

    // 代码/命令执行
    ExecuteCode(contextID string, code string) (<-chan ProcessOutput, error)
    ExecuteCommand(contextID string, cmd string) (<-chan ProcessOutput, error)
}
```

---

## 五、实现架构

### 5.0 MultiplexedChannel（多路复用通道）

设计目标：支持多个客户端订阅同一个进程的输出流（类似E2B的实现）。

```go
// MultiplexedChannel 多路复用通道
// 支持多个订阅者订阅同一个事件流
type MultiplexedChannel[T any] struct {
    mu          sync.RWMutex
    Source      chan T                 // 事件源
    subscribers []chan T               // 订阅者列表
    bufferSize  int                    // 每个订阅者的缓冲区大小
    closed      bool                   // 是否已关闭
}

// NewMultiplexedChannel 创建多路复用通道
func NewMultiplexedChannel[T any](bufferSize int) *MultiplexedChannel[T] {
    mc := &MultiplexedChannel[T]{
        Source:      make(chan T, bufferSize),
        subscribers: make([]chan T, 0),
        bufferSize:  bufferSize,
    }

    // 启动分发goroutine
    go mc.dispatch()

    return mc
}

// dispatch 分发事件到所有订阅者
func (mc *MultiplexedChannel[T]) dispatch() {
    for event := range mc.Source {
        mc.mu.RLock()
        // 发送到所有订阅者
        for _, sub := range mc.subscribers {
            select {
            case sub <- event:
                // 成功发送
            default:
                // 订阅者缓冲区满了，丢弃事件
                log.Printf("subscriber buffer full, dropping event")
            }
        }
        mc.mu.RUnlock()
    }

    // Source关闭，关闭所有订阅者
    mc.mu.Lock()
    defer mc.mu.Unlock()
    for _, sub := range mc.subscribers {
        close(sub)
    }
    mc.closed = true
}

// Fork 创建新的订阅
// 返回：订阅通道和取消订阅函数
func (mc *MultiplexedChannel[T]) Fork() (<-chan T, func()) {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    if mc.closed {
        return nil, func() {}
    }

    // 创建订阅者通道
    sub := make(chan T, mc.bufferSize)
    mc.subscribers = append(mc.subscribers, sub)

    // 取消订阅函数
    cancel := func() {
        mc.Unsubscribe(sub)
    }

    return sub, cancel
}

// Unsubscribe 取消订阅
func (mc *MultiplexedChannel[T]) Unsubscribe(sub chan T) {
    mc.mu.Lock()
    defer mc.mu.Unlock()

    for i, s := range mc.subscribers {
        if s == sub {
            // 删除订阅者
            mc.subscribers = append(mc.subscribers[:i], mc.subscribers[i+1:]...)
            close(sub)
            return
        }
    }
}

// Publish 发布事件（通过Source通道）
func (mc *MultiplexedChannel[T]) Publish(event T) {
    mc.Source <- event
}

// Close 关闭多路复用通道
func (mc *MultiplexedChannel[T]) Close() {
    close(mc.Source)
}

// SubscriberCount 订阅者数量
func (mc *MultiplexedChannel[T]) SubscriberCount() int {
    mc.mu.RLock()
    defer mc.mu.RUnlock()
    return len(mc.subscribers)
}
```

**使用示例**：

```go
// 进程输出多路复用
type ProcessOutput struct {
    Timestamp time.Time
    Source    OutputSource
    Data      []byte
}

// 在BaseProcess中创建多路复用输出通道
type BaseProcess struct {
    // ...
    outputMultiplex *MultiplexedChannel[ProcessOutput]
    pty             *os.File
}

// 新建进程时创建多路复用通道
func NewBaseProcess(config ProcessConfig) *BaseProcess {
    return &BaseProcess{
        outputMultiplex: NewMultiplexedChannel[ProcessOutput](64),
        // ...
    }
}

// PTY读取goroutine将事件发布到多路复用通道
go func() {
    for {
        buf := make([]byte, 4096)
        n, err := pty.Read(buf)
        if n > 0 {
            bp.outputMultiplex.Publish(ProcessOutput{
                Timestamp: time.Now(),
                Source:    OutputSourcePTY,
                Data:      buf[:n],
            })
        }
        if err != nil {
            break
        }
    }
}()

// 客户端订阅进程输出
outputCh, cancel := bp.outputMultiplex.Fork()
defer cancel()

for output := range outputCh {
    fmt.Printf("Output: %s\n", output.Data)
}
```

### 5.1 进程工厂

```go
// ProcessFactory 进程工厂
type ProcessFactory struct{}

func (f *ProcessFactory) CreateProcess(config ProcessConfig) (Process, error) {
    switch config.Type {
    case ProcessTypeREPL:
        return f.createREPLProcess(config)
    case ProcessTypeShell:
        return f.createShellProcess(config)
    default:
        return nil, fmt.Errorf("unsupported process type: %s", config.Type)
    }
}

func (f *ProcessFactory) createREPLProcess(config ProcessConfig) (Process, error) {
    lang := config.Language
    if lang == "" {
        lang = "python"  // 默认Python
    }

    switch lang {
    case "python", "python3":
        return NewPythonREPL(config)
    case "javascript", "node", "nodejs":
        return NewNodeREPL(config)
    case "typescript", "ts":
        return NewTSNodeREPL(config)
    case "ruby":
        return NewRubyREPL(config)
    case "r":
        return NewRREPL(config)
    default:
        return nil, fmt.Errorf("unsupported REPL language: %s", lang)
    }
}

func (f *ProcessFactory) createShellProcess(config ProcessConfig) (Process, error) {
    shell := config.Language
    if shell == "" {
        shell = "bash"  // 默认bash
    }

    switch shell {
    case "bash":
        return NewBashShell(config)
    case "zsh":
        return NewZshShell(config)
    case "fish":
        return NewFishShell(config)
    default:
        return nil, fmt.Errorf("unsupported shell: %s", shell)
    }
}
```

### 5.2 REPL 进程实现（Python示例）

```go
// PythonREPL Python REPL实现
type PythonREPL struct {
    REPLProcess
}

func NewPythonREPL(config ProcessConfig) (*PythonREPL, error) {
    // 使用IPython
    cmd := exec.Command("ipython", "--simple-prompt", "-i", "--no-banner")
    cmd.Dir = config.CWD

    // 设置环境变量
    env := os.Environ()
    for k, v := range config.EnvVars {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }
    cmd.Env = env

    // 设置PTY大小
    ptySize := config.PTYSize
    if ptySize == nil {
        ptySize = &PTYSize{Rows: 24, Cols: 80}
    }

    // 启动PTY
    pty, err := pty.StartWithSize(cmd, &pty.Winsize{
        Rows: ptySize.Rows,
        Cols: ptySize.Cols,
    })
    if err != nil {
        return nil, err
    }

    return &PythonREPL{
        REPLProcess: REPLProcess{
            BaseProcess: BaseProcess{
                config: config,
                pty:    pty,
                output: make(chan ProcessOutput, 100),
            },
            language: "python",
            cmd:      cmd,
        },
    }, nil
}

// ExecuteCode 执行Python代码
func (p *PythonREPL) ExecuteCode(code string) (*ExecutionResult, error) {
    // 写入代码到PTY
    fmt.Fprintln(p.pty, code)

    // 读取输出直到下一个提示符
    result := &ExecutionResult{}
    buf := make([]byte, 4096)

    for {
        n, err := p.pty.Read(buf)
        if n > 0 {
            data := buf[:n]

            // 检查是否是提示符（IPython的提示符）
            if p.detectPrompt(data) {
                break
            }

            result.Output = append(result.Output, data...)
        }
        if err != nil {
            if err == io.EOF {
                break
            }
            return nil, err
        }
    }

    return result, nil
}

// detectPrompt 检测IPython提示符
func (p *PythonREPL) detectPrompt(data []byte) bool {
    patterns := []string{
        "In [",     // IPython输入提示
        "Out[",     // IPython输出提示
        "...:",     // 续行提示
        ">>> ",     // 标准Python提示
    }

    str := string(data)
    for _, p := range patterns {
        if strings.Contains(str, p) {
            return true
        }
    }
    return false
}

// GetVariables 获取当前所有变量
func (p *PythonREPL) GetVariables() map[string]interface{} {
    result, err := p.ExecuteCode(
        "import json; print(json.dumps({k: repr(v) for k, v in globals().items() if not k.startswith('_')}))",
    )

    if err != nil {
        return nil
    }

    // 解析JSON输出
    var vars map[string]interface{}
    lines := strings.Split(string(result.Output), "\n")
    for _, line := range lines {
        if err := json.Unmarshal([]byte(line), &vars); err == nil {
            return vars
        }
    }

    return nil
}
```

### 5.3 Shell 进程实现（Bash示例）

```go
// BashShell Bash Shell实现
type BashShell struct {
    ShellProcess
}

func NewBashShell(config ProcessConfig) (*BashShell, error) {
    // 启动交互式Bash
    cmd := exec.Command("bash", "--interactive", "--login", "--norc", "--noprofile")
    cmd.Dir = config.CWD

    // 设置环境变量
    env := os.Environ()
    for k, v := range config.EnvVars {
        env = append(env, fmt.Sprintf("%s=%s", k, v))
    }
    term := config.Term
    if term == "" {
        term = "xterm-256color"
    }
    env = append(env, fmt.Sprintf("TERM=%s", term))
    cmd.Env = env

    // 设置PTY大小
    ptySize := config.PTYSize
    if ptySize == nil {
        ptySize = &PTYSize{Rows: 24, Cols: 80}
    }

    // 启动PTY
    pty, err := pty.StartWithSize(cmd, &pty.Winsize{
        Rows: ptySize.Rows,
        Cols: ptySize.Cols,
    })
    if err != nil {
        return nil, err
    }

    shell := &BashShell{
        ShellProcess: ShellProcess{
            BaseProcess: BaseProcess{
                config: config,
                pty:    pty,
                output: make(chan ProcessOutput, 100),
            },
            shellType: "bash",
            cmd:       cmd,
            pty:       pty,
        },
    }

    // 设置自定义提示符（便于检测命令完成）
    shell.SetPrompt("SANDBOX0_PROMPT>>> ")

    return shell, nil
}

// ExecuteCommand 在Shell中执行命令
func (s *BashShell) ExecuteCommand(cmd string) (*ExecutionResult, error) {
    result := &ExecutionResult{}

    // 写入命令（带换行）
    fmt.Fprintln(s.pty, cmd)

    // 读取输出直到下一个提示符
    buf := make([]byte, 4096)
    output := []byte{}

    for {
        n, err := s.pty.Read(buf)
        if n > 0 {
            data := buf[:n]

            // 检查是否是提示符（命令完成）
            if bytes.Contains(data, []byte(s.prompt)) {
                break
            }

            output = append(output, data...)
        }
        if err != nil {
            if err == io.EOF {
                break
            }
            return nil, err
        }
    }

    result.Output = output
    return result, nil
}

// ResizeTerminal 调整终端大小
func (s *BashShell) ResizeTerminal(size PTYSize) error {
    return pty.Setsize(s.pty, &pty.Winsize{
        Rows: size.Rows,
        Cols: size.Cols,
    })
}
```

### 5.4 Context 实现

```go
// NewContext 创建Context
func NewContext(config ProcessConfig) (*Context, error) {
    factory := &ProcessFactory{}
    process, err := factory.CreateProcess(config)
    if err != nil {
        return nil, err
    }

    if err := process.Start(); err != nil {
        return nil, err
    }

    return &Context{
        ID:          generateUUID(),
        Type:        config.Type,
        Language:    config.Language,
        CWD:         config.CWD,
        EnvVars:     config.EnvVars,
        MainProcess: process,
        CreatedAt:   time.Now(),
        UpdatedAt:   time.Now(),
    }, nil
}

// ExecuteCode 在REPL Context中执行代码
func (ctx *Context) ExecuteCode(code string) (<-chan ProcessOutput, error) {
    if ctx.Type != ProcessTypeREPL {
        return nil, fmt.Errorf("not a REPL context")
    }

    repl, ok := ctx.MainProcess.(interface{ ExecuteCode(string) (*ExecutionResult, error) })
    if !ok {
        return nil, fmt.Errorf("main process does not support ExecuteCode")
    }

    outputCh := make(chan ProcessOutput, 10)

    go func() {
        defer close(outputCh)

        result, err := repl.ExecuteCode(code)

        if len(result.Output) > 0 {
            outputCh <- ProcessOutput{
                Timestamp: time.Now(),
                Source:    OutputSourcePTY,
                Data:      result.Output,
            }
        }

        if err != nil {
            outputCh <- ProcessOutput{
                Timestamp: time.Now(),
                Source:    OutputSourceStderr,
                Data:      []byte(err.Error()),
            }
        }

        outputCh <- ProcessOutput{
            Timestamp: time.Now(),
            Source:    OutputSourcePTY,
            Data:      []byte("END_OF_EXECUTION"),
        }
    }()

    return outputCh, nil
}

// ExecuteCommand 在Shell Context中执行命令
func (ctx *Context) ExecuteCommand(cmd string) (<-chan ProcessOutput, error) {
    shell, ok := ctx.MainProcess.(interface{ ExecuteCommand(string) (*ExecutionResult, error) })
    if !ok {
        return nil, fmt.Errorf("main process does not support ExecuteCommand")
    }

    outputCh := make(chan ProcessOutput, 10)

    go func() {
        defer close(outputCh)

        result, err := shell.ExecuteCommand(cmd)

        if len(result.Output) > 0 {
            outputCh <- ProcessOutput{
                Timestamp: time.Now(),
                Source:    OutputSourcePTY,
                Data:      result.Output,
            }
        }

        if err != nil {
            outputCh <- ProcessOutput{
                Timestamp: time.Now(),
                Source:    OutputSourceStderr,
                Data:      []byte(err.Error()),
            }
        }

        outputCh <- ProcessOutput{
            Timestamp: time.Now(),
            Source:    OutputSourcePTY,
            Data:      []byte("END_OF_EXECUTION"),
        }
    }()

    return outputCh, nil
}
```

---

## 六、进程生命周期

```
┌─────────────────────────────────────────────────────────────────┐
│                     Process Lifecycle                           │
└─────────────────────────────────────────────────────────────────┘

  Created
     │
     ▼
  Starting  ←───┐
     │          │ Restart
     ▼          │
  Running  ─────┘
     │
     ├─► Stopped (正常退出)
     │
     ├─► Killed (被Kill)
     │
     └─► Crashed (崩溃)

状态转换:
- Starting → Running: 进程成功启动
- Running → Stopped: 进程正常退出(exitCode = 0)
- Running → Killed: 收到SIGKILL信号
- Running → Crashed: 进程异常退出(exitCode != 0)
- Stopped → Running: Restart操作（变量/状态会丢失，需手动恢复）
```

---

## 七、使用场景示例

### 7.1 Python 数据分析

```go
// 创建Python REPL Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     ProcessTypeREPL,
    Language: "python",
    CWD:      "/home/user/project",
})

// 执行代码（状态保持）
outputCh, _ := ctx.ExecuteCode("import pandas as pd")
outputCh, _ = ctx.ExecuteCode("df = pd.read_csv('data.csv')")
outputCh, _ = ctx.ExecuteCode("print(df.shape)")  // df变量仍然存在
```

### 7.2 Node.js 开发

```go
// 创建Node.js REPL Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     ProcessTypeREPL,
    Language: "node",
    CWD:      "/home/user/webapp",
})

ctx.ExecuteCode("const fs = require('fs')")

// 或者创建Shell Context执行npm命令
shellCtx, _ := manager.CreateContext(ProcessConfig{
    Type:     ProcessTypeShell,
    Language: "bash",
    CWD:      "/home/user/webapp",
})

// 像真实终端一样操作
shellCtx.ExecuteCommand("npm install")
shellCtx.ExecuteCommand("export NODE_ENV=production")
shellCtx.ExecuteCommand("npm run build")
shellCtx.ExecuteCommand("npm run dev &")  // 后台运行
```

### 7.3 交互式编辑

```go
// 创建Shell Context
ctx, _ := manager.CreateContext(ProcessConfig{
    Type:     ProcessTypeShell,
    Language: "bash",
    CWD:      "/home/user/project",
})

// 启动vim
ctx.ExecuteCommand("vim main.py")

// 通过WebSocket处理用户输入
for userInput := range userInputChannel {
    ctx.MainProcess.WriteInput(userInput)
}

// 调整终端大小
shell := ctx.MainProcess.(*BashShell)
shell.ResizeTerminal(PTYSize{Rows: 30, Cols: 100})
```

### 7.4 数据管道

```go
// Shell天然支持管道
ctx.ExecuteCommand("cat data.csv | grep 'error' | wc -l")

// 后台任务
ctx.ExecuteCommand("npm run dev &")

// 后台任务输出
ctx.ExecuteCommand("jobs")
ctx.ExecuteCommand("fg %1")  // 把后台任务调到前台
```

---

## 八、清理策略

```go
// Context清理
func (ctx *Context) Cleanup() error {
    // Stop main process
    // Note: Do NOT clean files here. Pod deletion will handle cleanup.
    // Persistent volume files must NOT be deleted.
    return ctx.MainProcess.Stop()
}
```

---

## 九、安全性考虑

### 9.1 命令验证

```go
// 基本命令验证
func validateCommand(cmd string) error {
    // 检查危险命令
    dangerous := []string{"rm -rf /", "mkfs", "dd if=/dev/zero"}
    for _, d := range dangerous {
        if strings.Contains(cmd, d) {
            return fmt.Errorf("dangerous command detected")
        }
    }
    return nil
}
```

---

## 十、错误处理

```go
type ProcessError struct {
    Code    string
    Message string
    PID     int
    Context string
}

const (
    ErrProcessNotFound    = "PROCESS_NOT_FOUND"
    ErrProcessStartFailed = "PROCESS_START_FAILED"
    ErrProcessKilled      = "PROCESS_KILLED"
    ErrProcessCrashed     = "PROCESS_CRASHED"
    ErrInvalidCommand     = "INVALID_COMMAND"
    ErrPermissionDenied   = "PERMISSION_DENIED"
)
```

---

## 十一、与 E2B 的兼容性

| E2B概念 | Sandbox0对应 |
|---------|--------------|
| Sandbox | Context |
| Kernel | REPL Process |
| Commands.run() | Shell Process.ExecuteCommand() |
| PTY.create() | Shell Process (PTY默认启用) |
| Code Interpreter | REPL Process.ExecuteCode() |
