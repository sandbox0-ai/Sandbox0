package manager

import "context"

// ManagerClient defines the interface for interacting with Manager service
// All methods are based on HTTP API defined in infra/spec/manager/manager.md
type ManagerClient interface {
	// Sandbox Management

	// ClaimSandbox claims an available sandbox or creates a new one
	// POST /api/v1/sandboxes/claim
	ClaimSandbox(ctx context.Context, req *ClaimSandboxRequest) (*ClaimSandboxResponse, error)

	// GetSandbox retrieves a sandbox by ID
	// GET /api/v1/sandboxes/{id}
	GetSandbox(ctx context.Context, id string) (*Sandbox, error)

	// ListSandboxes lists all sandboxes (optionally filtered by team/user)
	// GET /api/v1/sandboxes
	ListSandboxes(ctx context.Context, teamID, userID string) ([]*Sandbox, error)

	// GetSandboxStatus gets the status of a sandbox
	// GET /api/v1/sandboxes/{id}/status
	GetSandboxStatus(ctx context.Context, id string) (*SandboxStatusResponse, error)

	// UpdateSandbox updates a sandbox configuration
	// PATCH /api/v1/sandboxes/{id}
	UpdateSandbox(ctx context.Context, id string, req *UpdateSandboxRequest) error

	// DeleteSandbox deletes a sandbox
	// DELETE /api/v1/sandboxes/{id}
	DeleteSandbox(ctx context.Context, id string) error

	// PauseSandbox pauses a sandbox (resources freed but kept)
	// POST /api/v1/sandboxes/{id}/pause
	PauseSandbox(ctx context.Context, id string) error

	// ResumeSandbox resumes a paused sandbox
	// POST /api/v1/sandboxes/{id}/resume
	ResumeSandbox(ctx context.Context, id string) error

	// RefreshSandbox refreshes the TTL of a sandbox
	// POST /api/v1/sandboxes/{id}/refresh
	RefreshSandbox(ctx context.Context, id string) error

	// Template Management

	// CreateTemplate creates a new sandbox template
	// POST /api/v1/templates
	CreateTemplate(ctx context.Context, req *CreateTemplateRequest) (*Template, error)

	// GetTemplate retrieves a template by ID
	// GET /api/v1/templates/{id}
	GetTemplate(ctx context.Context, id string) (*Template, error)

	// ListTemplates lists all templates
	// GET /api/v1/templates
	ListTemplates(ctx context.Context) ([]*Template, error)

	// UpdateTemplate updates a template
	// PUT /api/v1/templates/{id}
	UpdateTemplate(ctx context.Context, id string, req *UpdateTemplateRequest) error

	// DeleteTemplate deletes a template
	// DELETE /api/v1/templates/{id}
	DeleteTemplate(ctx context.Context, id string) error

	// WarmPool warms up the pool for a template
	// POST /api/v1/templates/{id}/pool/warm
	WarmPool(ctx context.Context, id string, req *WarmPoolRequest) (*WarmPoolResponse, error)

	// Volume Management

	// CreateVolume creates a new volume
	// POST /api/v1/volumes
	CreateVolume(ctx context.Context, req *CreateVolumeRequest) (*Volume, error)

	// GetVolume retrieves a volume by ID
	// GET /api/v1/volumes/{id}
	GetVolume(ctx context.Context, id string) (*Volume, error)

	// ListVolumes lists all volumes (optionally filtered by team)
	// GET /api/v1/volumes
	ListVolumes(ctx context.Context, teamID string) ([]*Volume, error)

	// DeleteVolume deletes a volume
	// DELETE /api/v1/volumes/{id}
	DeleteVolume(ctx context.Context, id string) error
}

// ManagerClientConfig contains configuration for ManagerClient
type ManagerClientConfig struct {
	// BaseURL is the base URL of the manager service (e.g., "http://localhost:8080")
	BaseURL string

	// Timeout is the default timeout for requests
	Timeout int
}
