package db

import (
	"encoding/json"
	"time"
)

// SandboxVolume represents a sandbox volume metadata stored in the database
type SandboxVolume struct {
	ID     string `json:"id"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	// SourceVolumeID references the volume this one was forked from.
	SourceVolumeID *string `json:"source_volume_id,omitempty"`

	// Volume Configuration
	CacheSize  string `json:"cache_size"`
	Prefetch   int    `json:"prefetch"`
	BufferSize string `json:"buffer_size"`
	Writeback  bool   `json:"writeback"`
	AccessMode string `json:"access_mode"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Snapshot represents a point-in-time copy of a SandboxVolume
type Snapshot struct {
	ID       string `json:"id"`
	VolumeID string `json:"volume_id"`
	TeamID   string `json:"team_id"`
	UserID   string `json:"user_id"`

	// JuiceFS metadata
	RootInode   int64 `json:"root_inode"`   // Snapshot root directory inode
	SourceInode int64 `json:"source_inode"` // Source volume root inode at snapshot time

	// Metadata
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`

	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// VolumeMount represents a volume mount point for cross-cluster coordination
type VolumeMount struct {
	ID        string `json:"id"`
	VolumeID  string `json:"volume_id"`
	ClusterID string `json:"cluster_id"`
	PodID     string `json:"pod_id"`

	LastHeartbeat time.Time        `json:"last_heartbeat"`
	MountedAt     time.Time        `json:"mounted_at"`
	MountOptions  *json.RawMessage `json:"mount_options,omitempty"`
}

// SnapshotCoordination tracks the state of a snapshot creation across clusters
type SnapshotCoordination struct {
	ID       string `json:"id"`
	VolumeID string `json:"volume_id"`

	// Will be filled after successful snapshot creation
	SnapshotID *string `json:"snapshot_id,omitempty"`

	// Coordination state
	Status         string `json:"status"` // pending, flushing, completed, failed, timeout
	ExpectedNodes  int    `json:"expected_nodes"`
	CompletedNodes int    `json:"completed_nodes"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Coordination status constants
const (
	CoordStatusPending   = "pending"
	CoordStatusFlushing  = "flushing"
	CoordStatusCompleted = "completed"
	CoordStatusFailed    = "failed"
	CoordStatusTimeout   = "timeout"
)

// FlushResponse represents a node's response to a flush request
type FlushResponse struct {
	ID        string `json:"id"`
	CoordID   string `json:"coord_id"`
	ClusterID string `json:"cluster_id"`
	PodID     string `json:"pod_id"`

	Success      bool       `json:"success"`
	FlushedAt    *time.Time `json:"flushed_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// BaseLayer represents a shared, read-only image layer extracted to JuiceFS
type BaseLayer struct {
	ID          string  `json:"id"`
	TeamID      string  `json:"team_id"`
	ImageRef    string  `json:"image_ref"`              // Original image reference (e.g., "python:3.11")
	ImageDigest *string `json:"image_digest,omitempty"` // Image digest after extraction
	LayerPath   string  `json:"layer_path"`             // JuiceFS path for the extracted layer
	SizeBytes   int64   `json:"size_bytes"`

	// Extraction status
	Status      string     `json:"status"` // pending, extracting, ready, failed
	ExtractedAt *time.Time `json:"extracted_at,omitempty"`
	LastError   string     `json:"last_error,omitempty"`

	// Access tracking
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	RefCount       int        `json:"ref_count"` // Number of sandboxes using this layer

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BaseLayer status constants
const (
	BaseLayerStatusPending    = "pending"
	BaseLayerStatusExtracting = "extracting"
	BaseLayerStatusReady      = "ready"
	BaseLayerStatusFailed     = "failed"
)

// RootfsSnapshot represents a point-in-time snapshot of a sandbox's rootfs upper layer
type RootfsSnapshot struct {
	ID        string `json:"id"`
	SandboxID string `json:"sandbox_id"`
	TeamID    string `json:"team_id"`

	// Layer references
	BaseLayerID   string `json:"base_layer_id"`   // Base layer this snapshot is based on
	UpperVolumeID string `json:"upper_volume_id"` // Volume ID containing the upperdir snapshot

	// JuiceFS metadata
	RootInode   int64 `json:"root_inode"`   // Snapshot root directory inode
	SourceInode int64 `json:"source_inode"` // Source upperdir root inode at snapshot time

	// Metadata
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	SizeBytes   int64            `json:"size_bytes"`
	Metadata    *json.RawMessage `json:"metadata,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// SandboxRootfs represents the rootfs configuration for a sandbox
type SandboxRootfs struct {
	SandboxID string `json:"sandbox_id"`
	TeamID    string `json:"team_id"`

	// Layer configuration
	BaseLayerID   string `json:"base_layer_id"`   // Base layer for lowerdir
	UpperVolumeID string `json:"upper_volume_id"` // Volume for upperdir

	// Overlay paths (relative to JuiceFS root)
	UpperPath string `json:"upper_path"` // Path to upperdir
	WorkPath  string `json:"work_path"`  // Path to overlay workdir

	// Snapshot tracking
	CurrentSnapshotID *string `json:"current_snapshot_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
