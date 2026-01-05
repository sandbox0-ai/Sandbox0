package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidKey    = errors.New("invalid api key")
	ErrExpiredKey    = errors.New("api key expired")
	ErrInactiveKey   = errors.New("api key inactive")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// Repository provides database access for internal-gateway
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new database repository
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Pool returns the underlying connection pool
func (r *Repository) Pool() *pgxpool.Pool {
	return r.pool
}

// ValidateAPIKey validates an API key and returns the associated auth context
func (r *Repository) ValidateAPIKey(ctx context.Context, keyValue string) (*APIKey, error) {
	// API key format: sb0_<team_id>_<random_secret>
	if !strings.HasPrefix(keyValue, "sb0_") {
		return nil, ErrInvalidKey
	}

	var key APIKey
	var rolesJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT id, key_value, team_id, created_by, name, type, roles, 
		       is_active, expires_at, last_used_at, usage_count, created_at, updated_at
		FROM api_keys
		WHERE key_value = $1
	`, keyValue).Scan(
		&key.ID, &key.KeyValue, &key.TeamID, &key.CreatedBy,
		&key.Name, &key.Type, &rolesJSON, &key.IsActive,
		&key.ExpiresAt, &key.LastUsed, &key.UsageCount,
		&key.CreatedAt, &key.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidKey
		}
		return nil, fmt.Errorf("query api key: %w", err)
	}

	// Parse roles
	if len(rolesJSON) > 0 {
		if err := json.Unmarshal(rolesJSON, &key.Roles); err != nil {
			return nil, fmt.Errorf("parse roles: %w", err)
		}
	}

	// Check if key is active
	if !key.IsActive {
		return nil, ErrInactiveKey
	}

	// Check if key is expired
	if time.Now().After(key.ExpiresAt) {
		return nil, ErrExpiredKey
	}

	// Update usage statistics (fire and forget)
	go func() {
		_, _ = r.pool.Exec(context.Background(), `
			UPDATE api_keys 
			SET last_used_at = NOW(), usage_count = usage_count + 1
			WHERE id = $1
		`, key.ID)
	}()

	return &key, nil
}

// GetTeam retrieves a team by ID
func (r *Repository) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	var team Team

	err := r.pool.QueryRow(ctx, `
		SELECT id, name, quota, is_active, created_at, updated_at
		FROM teams
		WHERE id = $1
	`, teamID).Scan(
		&team.ID, &team.Name, &team.Quota, &team.IsActive,
		&team.CreatedAt, &team.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query team: %w", err)
	}

	return &team, nil
}

// GetSandbox retrieves sandbox information for routing
func (r *Repository) GetSandbox(ctx context.Context, sandboxID string) (*Sandbox, error) {
	var sandbox Sandbox

	err := r.pool.QueryRow(ctx, `
		SELECT id, template_id, team_id, procd_address, status, expires_at, created_at
		FROM sandboxes
		WHERE id = $1
	`, sandboxID).Scan(
		&sandbox.ID, &sandbox.TemplateID, &sandbox.TeamID,
		&sandbox.ProcdAddress, &sandbox.Status, &sandbox.ExpiresAt, &sandbox.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query sandbox: %w", err)
	}

	return &sandbox, nil
}

// CreateAuditLog creates an audit log entry
func (r *Repository) CreateAuditLog(ctx context.Context, log *AuditLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (team_id, user_id, api_key_id, request_id, method, path, 
		                        status_code, latency_ms, user_agent, client_ip, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	`,
		log.TeamID, nullString(log.UserID), nullString(log.APIKeyID),
		log.RequestID, log.Method, log.Path, log.StatusCode,
		log.LatencyMs, log.UserAgent, log.ClientIP, log.Metadata,
	)

	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

// CheckQuota checks if the team has remaining quota for the specified resource
func (r *Repository) CheckQuota(ctx context.Context, teamID string, quotaType string) (bool, error) {
	// Get team quota
	var quotaJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT quota FROM teams WHERE id = $1
	`, teamID).Scan(&quotaJSON)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("query team quota: %w", err)
	}

	var quota Quota
	if err := json.Unmarshal(quotaJSON, &quota); err != nil {
		return false, fmt.Errorf("parse quota: %w", err)
	}

	// Get current usage (depends on quota type)
	switch quotaType {
	case "sandbox_count":
		var count int
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM sandboxes WHERE team_id = $1 AND status != 'terminated'
		`, teamID).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("count sandboxes: %w", err)
		}
		return count < quota.SandboxCount, nil

	case "api_calls":
		var count int
		err := r.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM audit_logs 
			WHERE team_id = $1 AND created_at > NOW() - INTERVAL '1 hour'
		`, teamID).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("count api calls: %w", err)
		}
		return count < quota.APICallsPerHour, nil

	default:
		return true, nil
	}
}

// IncrementRateLimit uses database row-level locking for rate limiting
func (r *Repository) IncrementRateLimit(ctx context.Context, teamID string, window time.Duration, limit int) (bool, error) {
	// Use advisory lock + counter table for distributed rate limiting
	var allowed bool

	err := r.pool.QueryRow(ctx, `
		WITH rate_limit AS (
			INSERT INTO rate_limits (team_id, window_start, request_count)
			VALUES ($1, DATE_TRUNC('second', NOW()), 1)
			ON CONFLICT (team_id, window_start)
			DO UPDATE SET request_count = rate_limits.request_count + 1
			RETURNING request_count
		)
		SELECT request_count <= $2 FROM rate_limit
	`, teamID, limit).Scan(&allowed)

	if err != nil {
		// If table doesn't exist, allow (graceful degradation)
		return true, nil
	}

	return allowed, nil
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
