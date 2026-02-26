package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("not found")

	// PostgreSQL error code for lock not available (55P03)
	// This is returned when FOR UPDATE NOWAIT fails to acquire a lock
	PostgreSQLErrLockNotAvailable = "55P03"
)

// IsLockContentionError checks if the error is a PostgreSQL lock contention error (55P03)
// This error is returned when FOR UPDATE NOWAIT fails to acquire a lock
func IsLockContentionError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == PostgreSQLErrLockNotAvailable
	}
	return false
}

// CoordinatorRepository defines the database operations needed by coordinator
// This interface is implemented by *Repository
type CoordinatorRepository interface {
	// Mount operations
	CreateMount(ctx context.Context, mount *VolumeMount) error
	UpdateMountHeartbeat(ctx context.Context, volumeID, clusterID, podID string) error
	DeleteMount(ctx context.Context, volumeID, clusterID, podID string) error
	GetActiveMounts(ctx context.Context, volumeID string, heartbeatTimeout int) ([]*VolumeMount, error)
	DeleteStaleMounts(ctx context.Context, heartbeatTimeout int) (int64, error)

	// Coordination operations
	CreateCoordination(ctx context.Context, coord *SnapshotCoordination) error
	GetCoordination(ctx context.Context, id string) (*SnapshotCoordination, error)
	UpdateCoordinationStatus(ctx context.Context, id, status string) error
	CreateFlushResponse(ctx context.Context, resp *FlushResponse) error
	CountCompletedFlushes(ctx context.Context, coordID string) (int, error)
	GetFlushResponses(ctx context.Context, coordID string) ([]*FlushResponse, error)
}

// Repository provides database access for storage-proxy
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

// DB interface for query execution (supports both pool and transaction)
type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// BeginTx starts a new transaction
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

// WithTx executes a function within a transaction
// If the function returns an error, the transaction is rolled back
// Otherwise, the transaction is committed
// Note: This function does not propagate panics to maintain service stability.
// Panics are logged and converted to errors. Caller code should not panic.
func (r *Repository) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Ensure transaction is always finalized
	committed := false
	defer func() {
		if !committed {
			// Rollback on error or panic, ignore rollback errors in defer
			_ = tx.Rollback(ctx)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	committed = true
	return nil
}

// CreateSandboxVolume creates a new sandbox volume record
func (r *Repository) CreateSandboxVolume(ctx context.Context, volume *SandboxVolume) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sandbox_volumes (
			id, team_id, user_id,
			source_volume_id,
			cache_size, prefetch, buffer_size, writeback, access_mode,
			created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4,
			$5, $6, $7, $8, $9,
			$10, $11
		)
	`,
		volume.ID, volume.TeamID, volume.UserID,
		volume.SourceVolumeID,
		volume.CacheSize, volume.Prefetch, volume.BufferSize, volume.Writeback, volume.AccessMode,
		volume.CreatedAt, volume.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create sandbox volume: %w", err)
	}

	return nil
}

// GetSandboxVolume retrieves a sandbox volume by ID
func (r *Repository) GetSandboxVolume(ctx context.Context, id string) (*SandboxVolume, error) {
	return r.getSandboxVolume(ctx, r.pool, id, false)
}

// GetSandboxVolumeForUpdate retrieves a sandbox volume with FOR UPDATE NOWAIT lock
// This prevents deadlocks by failing immediately if the row is already locked
// Use this within a transaction when you need to ensure exclusive access
func (r *Repository) GetSandboxVolumeForUpdate(ctx context.Context, tx pgx.Tx, id string) (*SandboxVolume, error) {
	return r.getSandboxVolume(ctx, tx, id, true)
}

// getSandboxVolume internal implementation supporting both locked and unlocked reads
func (r *Repository) getSandboxVolume(ctx context.Context, db DB, id string, forUpdate bool) (*SandboxVolume, error) {
	var v SandboxVolume

	query := `
		SELECT
			id, team_id, user_id,
			source_volume_id,
			cache_size, prefetch, buffer_size, writeback, access_mode,
			created_at, updated_at
		FROM sandbox_volumes
		WHERE id = $1
	`

	// Add FOR UPDATE NOWAIT to prevent blocking and detect conflicts immediately
	if forUpdate {
		query += " FOR UPDATE NOWAIT"
	}

	err := db.QueryRow(ctx, query, id).Scan(
		&v.ID, &v.TeamID, &v.UserID,
		&v.SourceVolumeID,
		&v.CacheSize, &v.Prefetch, &v.BufferSize, &v.Writeback, &v.AccessMode,
		&v.CreatedAt, &v.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query sandbox volume: %w", err)
	}

	return &v, nil
}

// UpdateSandboxVolume updates an existing sandbox volume
func (r *Repository) UpdateSandboxVolume(ctx context.Context, volume *SandboxVolume) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE sandbox_volumes SET
			cache_size = $2,
			prefetch = $3,
			buffer_size = $4,
			writeback = $5,
			access_mode = $6,
			updated_at = NOW()
		WHERE id = $1
	`,
		volume.ID,
		volume.CacheSize, volume.Prefetch, volume.BufferSize, volume.Writeback, volume.AccessMode,
	)

	if err != nil {
		return fmt.Errorf("update sandbox volume: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListSandboxVolumesByTeam retrieves all volumes for a team
func (r *Repository) ListSandboxVolumesByTeam(ctx context.Context, teamID string) ([]*SandboxVolume, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, team_id, user_id,
			source_volume_id,
			cache_size, prefetch, buffer_size, writeback, access_mode,
			created_at, updated_at
		FROM sandbox_volumes
		WHERE team_id = $1
		ORDER BY created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("query sandbox volumes: %w", err)
	}
	defer rows.Close()

	var volumes []*SandboxVolume
	for rows.Next() {
		var v SandboxVolume
		err := rows.Scan(
			&v.ID, &v.TeamID, &v.UserID,
			&v.SourceVolumeID,
			&v.CacheSize, &v.Prefetch, &v.BufferSize, &v.Writeback, &v.AccessMode,
			&v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan sandbox volume: %w", err)
		}
		volumes = append(volumes, &v)
	}

	return volumes, nil
}

// DeleteSandboxVolume deletes a sandbox volume record
func (r *Repository) DeleteSandboxVolume(ctx context.Context, id string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		DELETE FROM sandbox_volumes WHERE id = $1
	`, id)

	if err != nil {
		return fmt.Errorf("delete sandbox volume: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ============================================================
// Snapshot Repository Methods
// ============================================================

// CreateSnapshot creates a new snapshot record
func (r *Repository) CreateSnapshot(ctx context.Context, snapshot *Snapshot) error {
	return r.createSnapshot(ctx, r.pool, snapshot)
}

// CreateSnapshotTx creates a new snapshot record within a transaction
func (r *Repository) CreateSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot *Snapshot) error {
	return r.createSnapshot(ctx, tx, snapshot)
}

// createSnapshot internal implementation supporting both pool and transaction
func (r *Repository) createSnapshot(ctx context.Context, db DB, snapshot *Snapshot) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sandbox_volume_snapshots (
			id, volume_id, team_id, user_id,
			root_inode, source_inode,
			name, description, size_bytes,
			created_at, expires_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8, $9,
			$10, $11
		)
	`,
		snapshot.ID, snapshot.VolumeID, snapshot.TeamID, snapshot.UserID,
		snapshot.RootInode, snapshot.SourceInode,
		snapshot.Name, snapshot.Description, snapshot.SizeBytes,
		snapshot.CreatedAt, snapshot.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	return nil
}

// GetSnapshot retrieves a snapshot by ID
func (r *Repository) GetSnapshot(ctx context.Context, id string) (*Snapshot, error) {
	return r.getSnapshot(ctx, r.pool, id, false)
}

// GetSnapshotTx retrieves a snapshot by ID within a transaction
func (r *Repository) GetSnapshotTx(ctx context.Context, tx pgx.Tx, id string) (*Snapshot, error) {
	return r.getSnapshot(ctx, tx, id, false)
}

// GetSnapshotForUpdate retrieves a snapshot with FOR UPDATE NOWAIT lock
func (r *Repository) GetSnapshotForUpdate(ctx context.Context, tx pgx.Tx, id string) (*Snapshot, error) {
	return r.getSnapshot(ctx, tx, id, true)
}

// getSnapshot internal implementation supporting both locked and unlocked reads
func (r *Repository) getSnapshot(ctx context.Context, db DB, id string, forUpdate bool) (*Snapshot, error) {
	var s Snapshot

	query := `
		SELECT
			id, volume_id, team_id, user_id,
			root_inode, source_inode,
			name, description, size_bytes,
			created_at, expires_at
		FROM sandbox_volume_snapshots
		WHERE id = $1
	`

	if forUpdate {
		query += " FOR UPDATE NOWAIT"
	}

	err := db.QueryRow(ctx, query, id).Scan(
		&s.ID, &s.VolumeID, &s.TeamID, &s.UserID,
		&s.RootInode, &s.SourceInode,
		&s.Name, &s.Description, &s.SizeBytes,
		&s.CreatedAt, &s.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query snapshot: %w", err)
	}

	return &s, nil
}

// ListSnapshotsByVolume retrieves all snapshots for a volume
func (r *Repository) ListSnapshotsByVolume(ctx context.Context, volumeID string) ([]*Snapshot, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, volume_id, team_id, user_id,
			root_inode, source_inode,
			name, description, size_bytes,
			created_at, expires_at
		FROM sandbox_volume_snapshots
		WHERE volume_id = $1
		ORDER BY created_at DESC
	`, volumeID)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*Snapshot
	for rows.Next() {
		var s Snapshot
		err := rows.Scan(
			&s.ID, &s.VolumeID, &s.TeamID, &s.UserID,
			&s.RootInode, &s.SourceInode,
			&s.Name, &s.Description, &s.SizeBytes,
			&s.CreatedAt, &s.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}

	return snapshots, nil
}

// DeleteSnapshot deletes a snapshot record
func (r *Repository) DeleteSnapshot(ctx context.Context, id string) error {
	return r.deleteSnapshot(ctx, r.pool, id)
}

// DeleteSnapshotTx deletes a snapshot record within a transaction
func (r *Repository) DeleteSnapshotTx(ctx context.Context, tx pgx.Tx, id string) error {
	return r.deleteSnapshot(ctx, tx, id)
}

// deleteSnapshot internal implementation supporting both pool and transaction
func (r *Repository) deleteSnapshot(ctx context.Context, db DB, id string) error {
	cmdTag, err := db.Exec(ctx, `
		DELETE FROM sandbox_volume_snapshots WHERE id = $1
	`, id)

	if err != nil {
		return fmt.Errorf("delete snapshot: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ============================================================
// Volume Mount Repository Methods (for cross-cluster coordination)
// ============================================================

// CreateMount creates a volume mount record
func (r *Repository) CreateMount(ctx context.Context, mount *VolumeMount) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sandbox_volume_mounts (
			id, volume_id, cluster_id, pod_id,
			last_heartbeat, mounted_at, mount_options
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7
		)
		ON CONFLICT (volume_id, cluster_id, pod_id) 
		DO UPDATE SET last_heartbeat = $5, mount_options = $7
	`,
		mount.ID, mount.VolumeID, mount.ClusterID, mount.PodID,
		mount.LastHeartbeat, mount.MountedAt, mount.MountOptions,
	)

	if err != nil {
		return fmt.Errorf("create mount: %w", err)
	}

	return nil
}

// UpdateMountHeartbeat updates the heartbeat for a mount
func (r *Repository) UpdateMountHeartbeat(ctx context.Context, volumeID, clusterID, podID string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE sandbox_volume_mounts 
		SET last_heartbeat = NOW()
		WHERE volume_id = $1 AND cluster_id = $2 AND pod_id = $3
	`, volumeID, clusterID, podID)

	if err != nil {
		return fmt.Errorf("update mount heartbeat: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteMount deletes a mount record
func (r *Repository) DeleteMount(ctx context.Context, volumeID, clusterID, podID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM sandbox_volume_mounts 
		WHERE volume_id = $1 AND cluster_id = $2 AND pod_id = $3
	`, volumeID, clusterID, podID)

	if err != nil {
		return fmt.Errorf("delete mount: %w", err)
	}

	return nil
}

// GetActiveMounts retrieves active mounts for a volume (heartbeat within threshold)
func (r *Repository) GetActiveMounts(ctx context.Context, volumeID string, heartbeatTimeout int) ([]*VolumeMount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, volume_id, cluster_id, pod_id,
			last_heartbeat, mounted_at, mount_options
		FROM sandbox_volume_mounts
		WHERE volume_id = $1 
			AND last_heartbeat > NOW() - INTERVAL '1 second' * $2
		ORDER BY mounted_at DESC
	`, volumeID, heartbeatTimeout)
	if err != nil {
		return nil, fmt.Errorf("query active mounts: %w", err)
	}
	defer rows.Close()

	var mounts []*VolumeMount
	for rows.Next() {
		var m VolumeMount
		err := rows.Scan(
			&m.ID, &m.VolumeID, &m.ClusterID, &m.PodID,
			&m.LastHeartbeat, &m.MountedAt, &m.MountOptions,
		)
		if err != nil {
			return nil, fmt.Errorf("scan mount: %w", err)
		}
		mounts = append(mounts, &m)
	}

	return mounts, nil
}

// DeleteStaleMounts deletes mounts with expired heartbeats
func (r *Repository) DeleteStaleMounts(ctx context.Context, heartbeatTimeout int) (int64, error) {
	cmdTag, err := r.pool.Exec(ctx, `
		DELETE FROM sandbox_volume_mounts 
		WHERE last_heartbeat < NOW() - INTERVAL '1 second' * $1
	`, heartbeatTimeout)

	if err != nil {
		return 0, fmt.Errorf("delete stale mounts: %w", err)
	}

	return cmdTag.RowsAffected(), nil
}

// ============================================================
// Snapshot Coordination Repository Methods
// ============================================================

// CreateCoordination creates a snapshot coordination record
func (r *Repository) CreateCoordination(ctx context.Context, coord *SnapshotCoordination) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO snapshot_coordinations (
			id, volume_id, snapshot_id,
			status, expected_nodes, completed_nodes,
			created_at, updated_at, expires_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9
		)
	`,
		coord.ID, coord.VolumeID, coord.SnapshotID,
		coord.Status, coord.ExpectedNodes, coord.CompletedNodes,
		coord.CreatedAt, coord.UpdatedAt, coord.ExpiresAt,
	)

	if err != nil {
		return fmt.Errorf("create coordination: %w", err)
	}

	return nil
}

// GetCoordination retrieves a coordination by ID
func (r *Repository) GetCoordination(ctx context.Context, id string) (*SnapshotCoordination, error) {
	var c SnapshotCoordination

	err := r.pool.QueryRow(ctx, `
		SELECT
			id, volume_id, snapshot_id,
			status, expected_nodes, completed_nodes,
			created_at, updated_at, expires_at
		FROM snapshot_coordinations
		WHERE id = $1
	`, id).Scan(
		&c.ID, &c.VolumeID, &c.SnapshotID,
		&c.Status, &c.ExpectedNodes, &c.CompletedNodes,
		&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query coordination: %w", err)
	}

	return &c, nil
}

// UpdateCoordinationStatus updates the status of a coordination
func (r *Repository) UpdateCoordinationStatus(ctx context.Context, id, status string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE snapshot_coordinations 
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`, id, status)

	if err != nil {
		return fmt.Errorf("update coordination status: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateCoordinationSnapshotID sets the snapshot ID after successful creation
func (r *Repository) UpdateCoordinationSnapshotID(ctx context.Context, coordID, snapshotID string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE snapshot_coordinations 
		SET snapshot_id = $2, status = $3, updated_at = NOW()
		WHERE id = $1
	`, coordID, snapshotID, CoordStatusCompleted)

	if err != nil {
		return fmt.Errorf("update coordination snapshot_id: %w", err)
	}

	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// CreateFlushResponse creates a flush response record
func (r *Repository) CreateFlushResponse(ctx context.Context, resp *FlushResponse) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO snapshot_flush_responses (
			id, coord_id, cluster_id, pod_id,
			success, flushed_at, error_message
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7
		)
		ON CONFLICT (coord_id, cluster_id, pod_id) 
		DO UPDATE SET success = $5, flushed_at = $6, error_message = $7
	`,
		resp.ID, resp.CoordID, resp.ClusterID, resp.PodID,
		resp.Success, resp.FlushedAt, resp.ErrorMessage,
	)

	if err != nil {
		return fmt.Errorf("create flush response: %w", err)
	}

	return nil
}

// CountCompletedFlushes counts successful flush responses for a coordination
func (r *Repository) CountCompletedFlushes(ctx context.Context, coordID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM snapshot_flush_responses 
		WHERE coord_id = $1 AND success = true
	`, coordID).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("count completed flushes: %w", err)
	}

	return count, nil
}

// GetFlushResponses retrieves all flush responses for a coordination
func (r *Repository) GetFlushResponses(ctx context.Context, coordID string) ([]*FlushResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id, coord_id, cluster_id, pod_id,
			success, flushed_at, error_message
		FROM snapshot_flush_responses
		WHERE coord_id = $1
	`, coordID)
	if err != nil {
		return nil, fmt.Errorf("query flush responses: %w", err)
	}
	defer rows.Close()

	var responses []*FlushResponse
	for rows.Next() {
		var r FlushResponse
		err := rows.Scan(
			&r.ID, &r.CoordID, &r.ClusterID, &r.PodID,
			&r.Success, &r.FlushedAt, &r.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("scan flush response: %w", err)
		}
		responses = append(responses, &r)
	}

	return responses, nil
}

// ============================================================
// Base Layer Repository Methods
// ============================================================

// CreateBaseLayer creates a new base layer record
func (r *Repository) CreateBaseLayer(ctx context.Context, layer *BaseLayer) error {
	return r.createBaseLayer(ctx, r.pool, layer)
}

// CreateBaseLayerTx creates a new base layer record within a transaction
func (r *Repository) CreateBaseLayerTx(ctx context.Context, tx pgx.Tx, layer *BaseLayer) error {
	return r.createBaseLayer(ctx, tx, layer)
}

func (r *Repository) createBaseLayer(ctx context.Context, db DB, layer *BaseLayer) error {
	_, err := db.Exec(ctx, `
		INSERT INTO base_layers (
			id, team_id, image_ref, image_digest, layer_path, size_bytes,
			status, extracted_at, last_error, last_accessed_at, ref_count,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`,
		layer.ID, layer.TeamID, layer.ImageRef, layer.ImageDigest, layer.LayerPath, layer.SizeBytes,
		layer.Status, layer.ExtractedAt, layer.LastError, layer.LastAccessedAt, layer.RefCount,
		layer.CreatedAt, layer.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create base layer: %w", err)
	}
	return nil
}

// GetBaseLayer retrieves a base layer by ID
func (r *Repository) GetBaseLayer(ctx context.Context, id string) (*BaseLayer, error) {
	var layer BaseLayer
	err := r.pool.QueryRow(ctx, `
		SELECT
			id, team_id, image_ref, image_digest, layer_path, size_bytes,
			status, extracted_at, last_error, last_accessed_at, ref_count,
			created_at, updated_at
		FROM base_layers
		WHERE id = $1
	`, id).Scan(
		&layer.ID, &layer.TeamID, &layer.ImageRef, &layer.ImageDigest, &layer.LayerPath, &layer.SizeBytes,
		&layer.Status, &layer.ExtractedAt, &layer.LastError, &layer.LastAccessedAt, &layer.RefCount,
		&layer.CreatedAt, &layer.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query base layer: %w", err)
	}
	return &layer, nil
}

// GetBaseLayerByImageRef retrieves a base layer by team and image reference
func (r *Repository) GetBaseLayerByImageRef(ctx context.Context, teamID, imageRef string) (*BaseLayer, error) {
	var layer BaseLayer
	err := r.pool.QueryRow(ctx, `
		SELECT
			id, team_id, image_ref, image_digest, layer_path, size_bytes,
			status, extracted_at, last_error, last_accessed_at, ref_count,
			created_at, updated_at
		FROM base_layers
		WHERE team_id = $1 AND image_ref = $2
	`, teamID, imageRef).Scan(
		&layer.ID, &layer.TeamID, &layer.ImageRef, &layer.ImageDigest, &layer.LayerPath, &layer.SizeBytes,
		&layer.Status, &layer.ExtractedAt, &layer.LastError, &layer.LastAccessedAt, &layer.RefCount,
		&layer.CreatedAt, &layer.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query base layer by image ref: %w", err)
	}
	return &layer, nil
}

// ListBaseLayers retrieves base layers with optional filtering
func (r *Repository) ListBaseLayers(ctx context.Context, teamID, status string, limit, offset int) ([]*BaseLayer, int, error) {
	// Build query conditions
	conditions := []string{"1=1"}
	args := []any{}
	argIdx := 1

	if teamID != "" {
		conditions = append(conditions, fmt.Sprintf("team_id = $%d", argIdx))
		args = append(args, teamID)
		argIdx++
	}
	if status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM base_layers WHERE %s", joinConditions(conditions))
	var total int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count base layers: %w", err)
	}

	// Query with pagination
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	query := fmt.Sprintf(`
		SELECT
			id, team_id, image_ref, image_digest, layer_path, size_bytes,
			status, extracted_at, last_error, last_accessed_at, ref_count,
			created_at, updated_at
		FROM base_layers
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, joinConditions(conditions), argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query base layers: %w", err)
	}
	defer rows.Close()

	var layers []*BaseLayer
	for rows.Next() {
		var layer BaseLayer
		err := rows.Scan(
			&layer.ID, &layer.TeamID, &layer.ImageRef, &layer.ImageDigest, &layer.LayerPath, &layer.SizeBytes,
			&layer.Status, &layer.ExtractedAt, &layer.LastError, &layer.LastAccessedAt, &layer.RefCount,
			&layer.CreatedAt, &layer.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan base layer: %w", err)
		}
		layers = append(layers, &layer)
	}

	return layers, total, nil
}

// UpdateBaseLayerStatus updates the status of a base layer
func (r *Repository) UpdateBaseLayerStatus(ctx context.Context, id, status, lastError string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE base_layers
		SET status = $2, last_error = $3, updated_at = NOW()
		WHERE id = $1
	`, id, status, lastError)
	if err != nil {
		return fmt.Errorf("update base layer status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateBaseLayerExtraction updates extraction-related fields
func (r *Repository) UpdateBaseLayerExtraction(ctx context.Context, id, imageDigest, layerPath string, sizeBytes int64) error {
	cmdTag, err := r.pool.Exec(ctx, `
		UPDATE base_layers
		SET image_digest = $2, layer_path = $3, size_bytes = $4,
		    status = $5, extracted_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id, imageDigest, layerPath, sizeBytes, BaseLayerStatusReady)
	if err != nil {
		return fmt.Errorf("update base layer extraction: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementBaseLayerRef increments the reference count for a base layer
func (r *Repository) IncrementBaseLayerRef(ctx context.Context, id string) (int, error) {
	var newCount int
	err := r.pool.QueryRow(ctx, `
		UPDATE base_layers
		SET ref_count = ref_count + 1, last_accessed_at = NOW(), updated_at = NOW()
		WHERE id = $1
		RETURNING ref_count
	`, id).Scan(&newCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("increment base layer ref: %w", err)
	}
	return newCount, nil
}

// DecrementBaseLayerRef decrements the reference count for a base layer
func (r *Repository) DecrementBaseLayerRef(ctx context.Context, id string) (int, error) {
	var newCount int
	err := r.pool.QueryRow(ctx, `
		UPDATE base_layers
		SET ref_count = GREATEST(ref_count - 1, 0), updated_at = NOW()
		WHERE id = $1
		RETURNING ref_count
	`, id).Scan(&newCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("decrement base layer ref: %w", err)
	}
	return newCount, nil
}

// DeleteBaseLayer deletes a base layer record
func (r *Repository) DeleteBaseLayer(ctx context.Context, id string) error {
	cmdTag, err := r.pool.Exec(ctx, `
		DELETE FROM base_layers WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete base layer: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUnusedBaseLayers finds layers with no references for garbage collection
func (r *Repository) ListUnusedBaseLayers(ctx context.Context, minAgeSeconds int, maxCount int) ([]*BaseLayer, error) {
	query := `
		SELECT
			id, team_id, image_ref, image_digest, layer_path, size_bytes,
			status, extracted_at, last_error, last_accessed_at, ref_count,
			created_at, updated_at
		FROM base_layers
		WHERE ref_count = 0 AND status = $1
			AND (last_accessed_at IS NULL OR last_accessed_at < NOW() - INTERVAL '1 second' * $2)
		ORDER BY last_accessed_at ASC NULLS FIRST
		LIMIT $3
	`
	rows, err := r.pool.Query(ctx, query, BaseLayerStatusReady, minAgeSeconds, maxCount)
	if err != nil {
		return nil, fmt.Errorf("query unused base layers: %w", err)
	}
	defer rows.Close()

	var layers []*BaseLayer
	for rows.Next() {
		var layer BaseLayer
		err := rows.Scan(
			&layer.ID, &layer.TeamID, &layer.ImageRef, &layer.ImageDigest, &layer.LayerPath, &layer.SizeBytes,
			&layer.Status, &layer.ExtractedAt, &layer.LastError, &layer.LastAccessedAt, &layer.RefCount,
			&layer.CreatedAt, &layer.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan base layer: %w", err)
		}
		layers = append(layers, &layer)
	}
	return layers, nil
}

// ============================================================
// Sandbox Rootfs Repository Methods
// ============================================================

// CreateSandboxRootfs creates a new sandbox rootfs record
func (r *Repository) CreateSandboxRootfs(ctx context.Context, rootfs *SandboxRootfs) error {
	return r.createSandboxRootfs(ctx, r.pool, rootfs)
}

// CreateSandboxRootfsTx creates a new sandbox rootfs record within a transaction
func (r *Repository) CreateSandboxRootfsTx(ctx context.Context, tx pgx.Tx, rootfs *SandboxRootfs) error {
	return r.createSandboxRootfs(ctx, tx, rootfs)
}

// createSandboxRootfs internal implementation supporting both pool and transaction
func (r *Repository) createSandboxRootfs(ctx context.Context, db DB, rootfs *SandboxRootfs) error {
	_, err := db.Exec(ctx, `
		INSERT INTO sandbox_rootfs (
			sandbox_id, team_id, base_layer_id, upper_volume_id,
			upper_path, work_path, current_snapshot_id,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`,
		rootfs.SandboxID, rootfs.TeamID, rootfs.BaseLayerID, rootfs.UpperVolumeID,
		rootfs.UpperPath, rootfs.WorkPath, rootfs.CurrentSnapshotID,
		rootfs.CreatedAt, rootfs.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create sandbox rootfs: %w", err)
	}
	return nil
}

// GetSandboxRootfs retrieves rootfs info for a sandbox
func (r *Repository) GetSandboxRootfs(ctx context.Context, sandboxID string) (*SandboxRootfs, error) {
	return r.getSandboxRootfs(ctx, r.pool, sandboxID, false)
}

// GetSandboxRootfsTx retrieves rootfs info for a sandbox within a transaction
func (r *Repository) GetSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) (*SandboxRootfs, error) {
	return r.getSandboxRootfs(ctx, tx, sandboxID, false)
}

// GetSandboxRootfsForUpdate retrieves rootfs with FOR UPDATE NOWAIT lock
// This prevents deadlocks by failing immediately if the row is already locked
// Use this within a transaction when you need to ensure exclusive access
func (r *Repository) GetSandboxRootfsForUpdate(ctx context.Context, tx pgx.Tx, sandboxID string) (*SandboxRootfs, error) {
	return r.getSandboxRootfs(ctx, tx, sandboxID, true)
}

// getSandboxRootfs internal implementation supporting both locked and unlocked reads
func (r *Repository) getSandboxRootfs(ctx context.Context, db DB, sandboxID string, forUpdate bool) (*SandboxRootfs, error) {
	var rootfs SandboxRootfs

	query := `
		SELECT
			sandbox_id, team_id, base_layer_id, upper_volume_id,
			upper_path, work_path, current_snapshot_id,
			created_at, updated_at
		FROM sandbox_rootfs
		WHERE sandbox_id = $1
	`

	// Add FOR UPDATE NOWAIT to prevent blocking and detect conflicts immediately
	if forUpdate {
		query += " FOR UPDATE NOWAIT"
	}

	err := db.QueryRow(ctx, query, sandboxID).Scan(
		&rootfs.SandboxID, &rootfs.TeamID, &rootfs.BaseLayerID, &rootfs.UpperVolumeID,
		&rootfs.UpperPath, &rootfs.WorkPath, &rootfs.CurrentSnapshotID,
		&rootfs.CreatedAt, &rootfs.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query sandbox rootfs: %w", err)
	}
	return &rootfs, nil
}

// UpdateSandboxRootfsSnapshot updates the current snapshot ID for a sandbox
func (r *Repository) UpdateSandboxRootfsSnapshot(ctx context.Context, sandboxID, snapshotID string) error {
	return r.updateSandboxRootfsSnapshot(ctx, r.pool, sandboxID, snapshotID)
}

// UpdateSandboxRootfsSnapshotTx updates the current snapshot ID for a sandbox within a transaction
func (r *Repository) UpdateSandboxRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, sandboxID, snapshotID string) error {
	return r.updateSandboxRootfsSnapshot(ctx, tx, sandboxID, snapshotID)
}

// updateSandboxRootfsSnapshot internal implementation supporting both pool and transaction
func (r *Repository) updateSandboxRootfsSnapshot(ctx context.Context, db DB, sandboxID, snapshotID string) error {
	cmdTag, err := db.Exec(ctx, `
		UPDATE sandbox_rootfs
		SET current_snapshot_id = $2, updated_at = NOW()
		WHERE sandbox_id = $1
	`, sandboxID, snapshotID)
	if err != nil {
		return fmt.Errorf("update sandbox rootfs snapshot: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSandboxRootfs deletes a sandbox rootfs record
func (r *Repository) DeleteSandboxRootfs(ctx context.Context, sandboxID string) error {
	return r.deleteSandboxRootfs(ctx, r.pool, sandboxID)
}

// DeleteSandboxRootfsTx deletes a sandbox rootfs record within a transaction
func (r *Repository) DeleteSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) error {
	return r.deleteSandboxRootfs(ctx, tx, sandboxID)
}

// deleteSandboxRootfs internal implementation supporting both pool and transaction
func (r *Repository) deleteSandboxRootfs(ctx context.Context, db DB, sandboxID string) error {
	cmdTag, err := db.Exec(ctx, `
		DELETE FROM sandbox_rootfs WHERE sandbox_id = $1
	`, sandboxID)
	if err != nil {
		return fmt.Errorf("delete sandbox rootfs: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// Rootfs Snapshot Repository Methods
// ============================================================

// CreateRootfsSnapshot creates a new rootfs snapshot record
func (r *Repository) CreateRootfsSnapshot(ctx context.Context, snapshot *RootfsSnapshot) error {
	return r.createRootfsSnapshot(ctx, r.pool, snapshot)
}

// CreateRootfsSnapshotTx creates a new rootfs snapshot record within a transaction
func (r *Repository) CreateRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot *RootfsSnapshot) error {
	return r.createRootfsSnapshot(ctx, tx, snapshot)
}

// createRootfsSnapshot internal implementation supporting both pool and transaction
func (r *Repository) createRootfsSnapshot(ctx context.Context, db DB, snapshot *RootfsSnapshot) error {
	_, err := db.Exec(ctx, `
		INSERT INTO rootfs_snapshots (
			id, sandbox_id, team_id, base_layer_id, upper_volume_id,
			root_inode, source_inode, name, description, size_bytes, metadata,
			created_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
	`,
		snapshot.ID, snapshot.SandboxID, snapshot.TeamID, snapshot.BaseLayerID, snapshot.UpperVolumeID,
		snapshot.RootInode, snapshot.SourceInode, snapshot.Name, snapshot.Description, snapshot.SizeBytes, snapshot.Metadata,
		snapshot.CreatedAt, snapshot.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create rootfs snapshot: %w", err)
	}
	return nil
}

// GetRootfsSnapshot retrieves a rootfs snapshot by ID
func (r *Repository) GetRootfsSnapshot(ctx context.Context, id string) (*RootfsSnapshot, error) {
	return r.getRootfsSnapshot(ctx, r.pool, id, false)
}

// GetRootfsSnapshotTx retrieves a rootfs snapshot by ID within a transaction
func (r *Repository) GetRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) (*RootfsSnapshot, error) {
	return r.getRootfsSnapshot(ctx, tx, id, false)
}

// GetRootfsSnapshotForUpdate retrieves a rootfs snapshot with FOR UPDATE NOWAIT lock
// This prevents deadlocks by failing immediately if the row is already locked
// Use this within a transaction when you need to ensure exclusive access
func (r *Repository) GetRootfsSnapshotForUpdate(ctx context.Context, tx pgx.Tx, id string) (*RootfsSnapshot, error) {
	return r.getRootfsSnapshot(ctx, tx, id, true)
}

// getRootfsSnapshot internal implementation supporting both locked and unlocked reads
func (r *Repository) getRootfsSnapshot(ctx context.Context, db DB, id string, forUpdate bool) (*RootfsSnapshot, error) {
	var snapshot RootfsSnapshot

	query := `
		SELECT
			id, sandbox_id, team_id, base_layer_id, upper_volume_id,
			root_inode, source_inode, name, description, size_bytes, metadata,
			created_at, expires_at
		FROM rootfs_snapshots
		WHERE id = $1
	`

	// Add FOR UPDATE NOWAIT to prevent blocking and detect conflicts immediately
	if forUpdate {
		query += " FOR UPDATE NOWAIT"
	}

	err := db.QueryRow(ctx, query, id).Scan(
		&snapshot.ID, &snapshot.SandboxID, &snapshot.TeamID, &snapshot.BaseLayerID, &snapshot.UpperVolumeID,
		&snapshot.RootInode, &snapshot.SourceInode, &snapshot.Name, &snapshot.Description, &snapshot.SizeBytes, &snapshot.Metadata,
		&snapshot.CreatedAt, &snapshot.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query rootfs snapshot: %w", err)
	}
	return &snapshot, nil
}

// ListRootfsSnapshotsBySandbox retrieves all snapshots for a sandbox
func (r *Repository) ListRootfsSnapshotsBySandbox(ctx context.Context, sandboxID string, limit, offset int) ([]*RootfsSnapshot, int, error) {
	// Count total
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM rootfs_snapshots WHERE sandbox_id = $1
	`, sandboxID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count rootfs snapshots: %w", err)
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			id, sandbox_id, team_id, base_layer_id, upper_volume_id,
			root_inode, source_inode, name, description, size_bytes, metadata,
			created_at, expires_at
		FROM rootfs_snapshots
		WHERE sandbox_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, sandboxID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query rootfs snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*RootfsSnapshot
	for rows.Next() {
		var s RootfsSnapshot
		err := rows.Scan(
			&s.ID, &s.SandboxID, &s.TeamID, &s.BaseLayerID, &s.UpperVolumeID,
			&s.RootInode, &s.SourceInode, &s.Name, &s.Description, &s.SizeBytes, &s.Metadata,
			&s.CreatedAt, &s.ExpiresAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan rootfs snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}
	return snapshots, total, nil
}

// DeleteRootfsSnapshot deletes a rootfs snapshot record
func (r *Repository) DeleteRootfsSnapshot(ctx context.Context, id string) error {
	return r.deleteRootfsSnapshot(ctx, r.pool, id)
}

// DeleteRootfsSnapshotTx deletes a rootfs snapshot record within a transaction
func (r *Repository) DeleteRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) error {
	return r.deleteRootfsSnapshot(ctx, tx, id)
}

// deleteRootfsSnapshot internal implementation supporting both pool and transaction
func (r *Repository) deleteRootfsSnapshot(ctx context.Context, db DB, id string) error {
	cmdTag, err := db.Exec(ctx, `
		DELETE FROM rootfs_snapshots WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete rootfs snapshot: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRootfsSnapshotsBySandbox deletes all snapshots for a sandbox
func (r *Repository) DeleteRootfsSnapshotsBySandbox(ctx context.Context, sandboxID string) (int64, error) {
	cmdTag, err := r.pool.Exec(ctx, `
		DELETE FROM rootfs_snapshots WHERE sandbox_id = $1
	`, sandboxID)
	if err != nil {
		return 0, fmt.Errorf("delete rootfs snapshots: %w", err)
	}
	return cmdTag.RowsAffected(), nil
}

// ListExpiredRootfsSnapshots retrieves all expired rootfs snapshots for garbage collection
// An expired snapshot has expires_at < NOW() and is eligible for cleanup
func (r *Repository) ListExpiredRootfsSnapshots(ctx context.Context, limit int) ([]*RootfsSnapshot, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			id, sandbox_id, team_id, base_layer_id, upper_volume_id,
			root_inode, source_inode, name, description, size_bytes, metadata,
			created_at, expires_at
		FROM rootfs_snapshots
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
		ORDER BY expires_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query expired rootfs snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*RootfsSnapshot
	for rows.Next() {
		var s RootfsSnapshot
		err := rows.Scan(
			&s.ID, &s.SandboxID, &s.TeamID, &s.BaseLayerID, &s.UpperVolumeID,
			&s.RootInode, &s.SourceInode, &s.Name, &s.Description, &s.SizeBytes, &s.Metadata,
			&s.CreatedAt, &s.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan expired rootfs snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}
	return snapshots, nil
}

// ListExpiredVolumeSnapshots retrieves all expired volume snapshots for garbage collection
// An expired snapshot has expires_at < NOW() and is eligible for cleanup
func (r *Repository) ListExpiredVolumeSnapshots(ctx context.Context, limit int) ([]*Snapshot, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			id, volume_id, team_id, user_id,
			root_inode, source_inode,
			name, description, size_bytes,
			created_at, expires_at
		FROM sandbox_volume_snapshots
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
		ORDER BY expires_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query expired volume snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []*Snapshot
	for rows.Next() {
		var s Snapshot
		err := rows.Scan(
			&s.ID, &s.VolumeID, &s.TeamID, &s.UserID,
			&s.RootInode, &s.SourceInode,
			&s.Name, &s.Description, &s.SizeBytes,
			&s.CreatedAt, &s.ExpiresAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan expired volume snapshot: %w", err)
		}
		snapshots = append(snapshots, &s)
	}
	return snapshots, nil
}

// Helper function to join conditions
func joinConditions(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := strings.Builder{}
	result.WriteString(conditions[0])
	for _, c := range conditions[1:] {
		result.WriteString(" AND ")
		result.WriteString(c)
	}
	return result.String()
}
