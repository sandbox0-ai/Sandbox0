package procd

import "time"

// ProcessType represents the type of process
type ProcessType string

const (
	ProcessTypeREPL  ProcessType = "repl"
	ProcessTypeShell ProcessType = "shell"
)

// CreateContextRequest represents a request to create a context
type CreateContextRequest struct {
	Type     ProcessType        `json:"type"`
	Language string             `json:"language"`
	CWD      string             `json:"cwd"`
	EnvVars  map[string]string  `json:"env_vars,omitempty"`
}

// Context represents a process context
type Context struct {
	ID          string     `json:"id"`
	Type        ProcessType `json:"type"`
	Language    string     `json:"language"`
	CWD         string     `json:"cwd"`
	MainProcess MainProcess `json:"main_process"`
	CreatedAt   time.Time  `json:"created_at"`
}

// MainProcess represents the main process info
type MainProcess struct {
	ID   string     `json:"id"`
	PID  int32      `json:"pid"`
	Type ProcessType `json:"type"`
}

// ExecuteRequest represents a request to execute code
type ExecuteRequest struct {
	Code string `json:"code"`
}

// ExecuteOutput represents output from execution
type ExecuteOutput struct {
	Type string `json:"type"` // "pty" or "end"
	Text string `json:"text,omitempty"`
}

// CommandRequest represents a request to execute a command
type CommandRequest struct {
	Command string `json:"command"`
}

// VolumeMountRequest represents a request to mount a volume
type VolumeMountRequest struct {
	SandboxID    string            `json:"sandbox_id"`
	MountPoint   string            `json:"mount_point"`
	ReadOnly     bool              `json:"read_only"`
	SnapshotID   string            `json:"snapshot_id,omitempty"`
	WarmupConfig *WarmupConfig     `json:"warmup_config,omitempty"`
}

// WarmupConfig represents warmup configuration
type WarmupConfig struct {
	Enabled      bool     `json:"enabled"`
	BaseLayerIDs []string `json:"base_layer_ids,omitempty"`
}

// VolumeMountResponse represents a response from mounting a volume
type VolumeMountResponse struct {
	VolumeID    string   `json:"volume_id"`
	MountPoint  string   `json:"mount_point"`
	LayerChain  []string `json:"layer_chain"`
	IsFromCache bool     `json:"is_from_cache"`
}

// VolumeStatus represents the status of a volume
type VolumeStatus struct {
	VolumeID   string    `json:"volume_id"`
	MountPoint string    `json:"mount_point"`
	Mounted    bool      `json:"mounted"`
	SandboxID  string    `json:"sandbox_id,omitempty"`
}

// SnapshotCreateRequest represents a request to create a snapshot
type SnapshotCreateRequest struct {
	VolumeID string `json:"volume_id"`
	Name     string `json:"name,omitempty"`
}

// Snapshot represents a volume snapshot
type Snapshot struct {
	ID        string    `json:"id"`
	VolumeID  string    `json:"volume_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// SnapshotRestoreRequest represents a request to restore a snapshot
type SnapshotRestoreRequest struct {
	VolumeID   string `json:"volume_id"`
	SnapshotID string `json:"snapshot_id"`
}

// NetworkPolicyMode represents network policy mode
type NetworkPolicyMode string

const (
	NetworkModeAllowAll   NetworkPolicyMode = "allow_all"
	NetworkModeWhitelist  NetworkPolicyMode = "whitelist"
	NetworkModeBlacklist  NetworkPolicyMode = "blacklist"
)

// NetworkPolicy represents a network policy
type NetworkPolicy struct {
	Mode    NetworkPolicyMode      `json:"mode"`
	Egress  *NetworkEgressPolicy   `json:"egress,omitempty"`
	Ingress *NetworkIngressPolicy  `json:"ingress,omitempty"`
	UpdatedAt *time.Time           `json:"updated_at,omitempty"`
}

// NetworkEgressPolicy represents egress network policy
type NetworkEgressPolicy struct {
	AllowCIDRs    []string `json:"allow_cidrs,omitempty"`
	AllowDomains  []string `json:"allow_domains,omitempty"`
	DenyCIDRs     []string `json:"deny_cidrs,omitempty"`
	TCPPort       int32    `json:"tcp_proxy_port,omitempty"`
}

// NetworkIngressPolicy represents ingress network policy
type NetworkIngressPolicy struct {
	AllowCIDRs   []string `json:"allow_cidrs,omitempty"`
	DenyCIDRs    []string `json:"deny_cidrs,omitempty"`
}

// FileContent represents file content
type FileContent struct {
	Content  string    `json:"content"`
	Size     int64     `json:"size"`
	Mode     string    `json:"mode"`
	ModTime  time.Time `json:"mod_time"`
}

// FileInfo represents file information
type FileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Type      string    `json:"type"` // "file", "dir", "symlink"
	Size      int64     `json:"size"`
	Mode      string    `json:"mode"`
	ModTime   time.Time `json:"mod_time"`
	IsLink    bool      `json:"is_link"`
	LinkTarget string   `json:"link_target,omitempty"`
}

// FileWriteRequest represents a request to write a file
type FileWriteRequest struct {
	Content string `json:"content"`
	Mode    string `json:"mode,omitempty"`
}

// MakeDirRequest represents a request to create a directory
type MakeDirRequest struct {
	Mode      string `json:"mode,omitempty"`
	Recursive bool   `json:"recursive"`
}

// FileMoveRequest represents a request to move a file
type FileMoveRequest struct {
	Src string `json:"src"`
	Dst string `json:"dst"`
}

// FileListResponse represents a response from listing a directory
type FileListResponse struct {
	Path    string      `json:"path"`
	Entries []FileInfo  `json:"entries"`
}

// FileWatchRequest represents a request to watch files
type FileWatchRequest struct {
	Action    string `json:"action"`    // "subscribe" or "unsubscribe"
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
	WatchID   string `json:"watch_id,omitempty"`
}

// FileWatchEvent represents a file watch event
type FileWatchEvent struct {
	Type      string    `json:"type"` // "create", "write", "remove", "rename", "chmod"
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// PreloadLayerRequest represents a request to preload a layer
type PreloadLayerRequest struct {
	LayerID   string `json:"layer_id"`
	S3Path    string `json:"s3_path"`
}
