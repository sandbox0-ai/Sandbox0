package db

import (
	"encoding/json"
	"time"
)

// AuthMethod defines the authentication method
type AuthMethod string

const (
	AuthMethodAPIKey   AuthMethod = "api_key"
	AuthMethodJWT      AuthMethod = "jwt"
	AuthMethodInternal AuthMethod = "internal"
)

// APIKey represents an API key stored in the database
type APIKey struct {
	ID         string    `json:"id"`
	KeyValue   string    `json:"key_value"`
	TeamID     string    `json:"team_id"`
	CreatedBy  string    `json:"created_by"`
	Name       string    `json:"name"`
	Type       string    `json:"type"` // 'user', 'service', 'internal'
	Roles      []string  `json:"roles"`
	IsActive   bool      `json:"is_active"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastUsed   time.Time `json:"last_used_at"`
	UsageCount int64     `json:"usage_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Team represents a team in the system
type Team struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Quota     json.RawMessage `json:"quota"`
	IsActive  bool            `json:"is_active"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// User represents a user in the system
type User struct {
	ID            string    `json:"id"`
	ExternalID    string    `json:"external_id"`
	Provider      string    `json:"provider"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	PrimaryTeamID string    `json:"primary_team_id"`
	Roles         []string  `json:"roles"`
	Permissions   []string  `json:"permissions"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID         int64           `json:"id"`
	TeamID     string          `json:"team_id"`
	UserID     string          `json:"user_id"`
	APIKeyID   string          `json:"api_key_id"`
	RequestID  string          `json:"request_id"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	StatusCode int             `json:"status_code"`
	LatencyMs  int             `json:"latency_ms"`
	UserAgent  string          `json:"user_agent"`
	ClientIP   string          `json:"client_ip"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Quota defines quota limits for a team
type Quota struct {
	SandboxCount           int `json:"sandbox_count"`
	SandboxCPUCores        int `json:"sandbox_cpu_cores"`
	SandboxMemoryMB        int `json:"sandbox_memory_mb"`
	SandboxVolumeStorageGB int `json:"sandboxvolume_storage_gb"`
	APICallsPerHour        int `json:"api_calls_per_hour"`
}

// QuotaUsage tracks current usage against quota
type QuotaUsage struct {
	TeamID                 string    `json:"team_id"`
	SandboxCount           int       `json:"sandbox_count"`
	SandboxCPUCores        int       `json:"sandbox_cpu_cores"`
	SandboxMemoryMB        int       `json:"sandbox_memory_mb"`
	SandboxVolumeStorageGB int       `json:"sandboxvolume_storage_gb"`
	APICallsThisHour       int       `json:"api_calls_this_hour"`
	LastUpdated            time.Time `json:"last_updated"`
}

// Sandbox represents a sandbox instance (for routing purposes)
type Sandbox struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"template_id"`
	TeamID       string    `json:"team_id"`
	ProcdAddress string    `json:"procd_address"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}
