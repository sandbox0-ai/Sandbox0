package manager

import "time"

// SandboxStatus represents the status of a sandbox
type SandboxStatus string

const (
	SandboxStatusStarting SandboxStatus = "starting"
	SandboxStatusRunning  SandboxStatus = "running"
	SandboxStatusPaused   SandboxStatus = "paused"
	SandboxStatusStopping SandboxStatus = "stopping"
	SandboxStatusStopped  SandboxStatus = "stopped"
	SandboxStatusError    SandboxStatus = "error"
)

// ClaimSandboxRequest represents a request to claim a sandbox
type ClaimSandboxRequest struct {
	TemplateID string                 `json:"template_id"`
	TeamID     string                 `json:"team_id"`
	UserID     string                 `json:"user_id"`
	SandboxID  string                 `json:"sandbox_id"`
	Config     *SandboxConfig         `json:"config,omitempty"`
}

// SandboxConfig represents sandbox configuration
type SandboxConfig struct {
	EnvVars      map[string]string     `json:"env_vars,omitempty"`
	TTL          int32                 `json:"ttl,omitempty"`
	Network      *NetworkPolicy        `json:"network,omitempty"`
}

// NetworkPolicy represents network policy (simplified for manager)
type NetworkPolicy struct {
	Mode   string                  `json:"mode"`
	Egress *NetworkEgressPolicy    `json:"egress,omitempty"`
}

// NetworkEgressPolicy represents egress network policy
type NetworkEgressPolicy struct {
	AllowCIDRs   []string `json:"allow_cidrs,omitempty"`
	AllowDomains []string `json:"allow_domains,omitempty"`
	DenyCIDRs    []string `json:"deny_cidrs,omitempty"`
}

// ClaimSandboxResponse represents a response from claiming a sandbox
type ClaimSandboxResponse struct {
	SandboxID   string        `json:"sandbox_id"`
	TemplateID  string        `json:"template_id"`
	Status      SandboxStatus `json:"status"`
	ProcdAddress string       `json:"procd_address"`
}

// Sandbox represents a sandbox instance
type Sandbox struct {
	ID           string        `json:"id"`
	TemplateID   string        `json:"template_id"`
	TeamID       string        `json:"team_id"`
	UserID       string        `json:"user_id"`
	Status       SandboxStatus `json:"status"`
	ProcdAddress string        `json:"procd_address"`
	Config       *SandboxConfig `json:"config,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	ExpiresAt    *time.Time    `json:"expires_at,omitempty"`
}

// SandboxStatusResponse represents the status of a sandbox
type SandboxStatusResponse struct {
	SandboxID   string        `json:"sandbox_id"`
	Status      SandboxStatus `json:"status"`
	Health      string        `json:"health"`
	ProcdReady  bool          `json:"procd_ready"`
}

// UpdateSandboxRequest represents a request to update a sandbox
type UpdateSandboxRequest struct {
	Config *SandboxConfig `json:"config,omitempty"`
	TTL    *int32         `json:"ttl,omitempty"`
}

// Template represents a sandbox template
type Template struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Spec        *TemplateSpec          `json:"spec"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TemplateSpec represents template specification (simplified)
type TemplateSpec struct {
	Description string              `json:"description"`
	MainContainer *ContainerSpec    `json:"main_container"`
	Resources   *ResourceQuota      `json:"resources"`
	Pool        *PoolStrategy       `json:"pool"`
}

// ContainerSpec represents container specification
type ContainerSpec struct {
	Image   string            `json:"image"`
	Command []string          `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	EnvVars map[string]string `json:"env_vars,omitempty"`
}

// ResourceQuota represents resource quota
type ResourceQuota struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	GPU    string `json:"gpu,omitempty"`
}

// PoolStrategy represents pool strategy
type PoolStrategy struct {
	MinIdle int32 `json:"min_idle"`
	MaxIdle int32 `json:"max_idle"`
	MaxPool int32 `json:"max_pool"`
}

// CreateTemplateRequest represents a request to create a template
type CreateTemplateRequest struct {
	Name string        `json:"name"`
	Spec *TemplateSpec `json:"spec"`
}

// UpdateTemplateRequest represents a request to update a template
type UpdateTemplateRequest struct {
	Spec *TemplateSpec `json:"spec"`
}

// WarmPoolRequest represents a request to warm the pool
type WarmPoolRequest struct {
	Count int32 `json:"count"`
}

// WarmPoolResponse represents a response from warming the pool
type WarmPoolResponse struct {
	TemplateID  string `json:"template_id"`
	WarmedCount int32  `json:"warmed_count"`
}

// Volume represents a volume
type Volume struct {
	ID          string                 `json:"id"`
	TeamID      string                 `json:"team_id"`
	Name        string                 `json:"name"`
	Config      *VolumeConfig          `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
}

// VolumeConfig represents volume configuration
type VolumeConfig struct {
	SizeGB     int32  `json:"size_gb"`
	StorageClass string `json:"storage_class"`
}

// CreateVolumeRequest represents a request to create a volume
type CreateVolumeRequest struct {
	TeamID string         `json:"team_id"`
	Name   string         `json:"name"`
	Config *VolumeConfig  `json:"config"`
}
