package procd

import "context"

// ProcdClient defines the interface for interacting with Procd service
// All methods are based on HTTP API defined in infra/spec/procd/procd.md
type ProcdClient interface {
	// Context Management

	// CreateContext creates a new process context (REPL or Shell)
	// POST /api/v1/contexts
	CreateContext(ctx context.Context, req *CreateContextRequest) (*Context, error)

	// GetContext retrieves a context by ID
	// GET /api/v1/contexts/{id}
	GetContext(ctx context.Context, id string) (*Context, error)

	// ListContexts lists all contexts
	// GET /api/v1/contexts
	ListContexts(ctx context.Context) ([]*Context, error)

	// DeleteContext deletes a context
	// DELETE /api/v1/contexts/{id}
	DeleteContext(ctx context.Context, id string) error

	// RestartContext restarts a context
	// POST /api/v1/contexts/{id}/restart
	RestartContext(ctx context.Context, id string) error

	// ExecuteCode executes code in a REPL context
	// Returns a channel of outputs for streaming SSE responses
	// POST /api/v1/contexts/{id}/execute
	ExecuteCode(ctx context.Context, contextID string, req *ExecuteRequest) (<-chan *ExecuteOutput, error)

	// ExecuteCommand executes a command in a Shell context
	// POST /api/v1/contexts/{id}/command
	ExecuteCommand(ctx context.Context, contextID string, req *CommandRequest) error

	// Volume Management

	// MountVolume mounts a volume to the sandbox
	// POST /api/v1/volumes/{volume_id}/mount
	MountVolume(ctx context.Context, volumeID string, req *VolumeMountRequest) (*VolumeMountResponse, error)

	// UnmountVolume unmounts a volume
	// POST /api/v1/volumes/{volume_id}/unmount
	UnmountVolume(ctx context.Context, volumeID string) error

	// GetVolumeStatus gets the status of a volume
	// GET /api/v1/volumes/{volume_id}
	GetVolumeStatus(ctx context.Context, volumeID string) (*VolumeStatus, error)

	// CreateSnapshot creates a snapshot of a volume
	// POST /api/v1/volumes/{volume_id}/snapshots
	CreateSnapshot(ctx context.Context, req *SnapshotCreateRequest) (*Snapshot, error)

	// ListSnapshots lists snapshots of a volume
	// GET /api/v1/volumes/{volume_id}/snapshots
	ListSnapshots(ctx context.Context, volumeID string) ([]*Snapshot, error)

	// RestoreSnapshot restores a volume to a snapshot
	// POST /api/v1/volumes/{volume_id}/restore
	RestoreSnapshot(ctx context.Context, req *SnapshotRestoreRequest) error

	// PreloadLayer preloads a layer for faster mounting
	// POST /api/v1/volumes/preload
	PreloadLayer(ctx context.Context, req *PreloadLayerRequest) error

	// Network Policy Management

	// GetNetworkPolicy gets the current network policy
	// GET /api/v1/network/policy
	GetNetworkPolicy(ctx context.Context) (*NetworkPolicy, error)

	// UpdateNetworkPolicy updates the network policy
	// PUT /api/v1/network/policy
	UpdateNetworkPolicy(ctx context.Context, policy *NetworkPolicy) error

	// ResetNetworkPolicy resets to default policy (allow-all)
	// POST /api/v1/network/policy/reset
	ResetNetworkPolicy(ctx context.Context) error

	// File Operations

	// ReadFile reads a file
	// GET /api/v1/files/*path
	ReadFile(ctx context.Context, path string) (*FileContent, error)

	// WriteFile writes a file
	// POST /api/v1/files/*path
	WriteFile(ctx context.Context, path string, req *FileWriteRequest) error

	// StatFile gets file/directory information
	// GET /api/v1/files/*path?stat=true
	StatFile(ctx context.Context, path string) (*FileInfo, error)

	// MakeDir creates a directory
	// POST /api/v1/files/*path?mkdir=true
	MakeDir(ctx context.Context, path string, req *MakeDirRequest) error

	// MoveFile moves/renames a file
	// POST /api/v1/files/move
	MoveFile(ctx context.Context, req *FileMoveRequest) error

	// ListFiles lists a directory
	// GET /api/v1/files/*path?list=true
	ListFiles(ctx context.Context, path string) (*FileListResponse, error)

	// DeleteFile deletes a file
	// DELETE /api/v1/files/*path
	DeleteFile(ctx context.Context, path string) error

	// WatchFiles watches a directory for changes
	// Returns a channel of file watch events
	// WS /api/v1/files/watch
	WatchFiles(ctx context.Context) (<-chan *FileWatchEvent, error)

	// Health Check

	// HealthCheck performs a health check
	// GET /healthz
	HealthCheck(ctx context.Context) (*HealthResponse, error)
}

// ProcdClientConfig contains configuration for ProcdClient
type ProcdClientConfig struct {
	// BaseURL is the base URL of the procd service (e.g., "http://localhost:8080")
	BaseURL string

	// Timeout is the default timeout for requests
	Timeout int
}
