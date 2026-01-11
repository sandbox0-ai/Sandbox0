package snapshot

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/config"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/volume"
	"github.com/sirupsen/logrus"
)

// Errors
var (
	ErrVolumeNotFound            = errors.New("volume not found")
	ErrSnapshotNotFound          = errors.New("snapshot not found")
	ErrSnapshotNotBelongToVolume = errors.New("snapshot does not belong to volume")
	ErrVolumeLocked              = errors.New("volume is locked for restore")
	ErrFlushFailed               = errors.New("flush failed on one or more nodes")
	ErrCloneFailed               = errors.New("clone operation failed")
)

// Manager handles snapshot operations for SandboxVolumes
type Manager struct {
	mu        sync.RWMutex
	locks     map[string]time.Time // volumeID -> lock acquired time
	repo      *db.Repository
	volMgr    *volume.Manager
	config    *config.Config
	logger    *logrus.Logger
	clusterID string
	podID     string
}

// NewManager creates a new snapshot manager
func NewManager(
	repo *db.Repository,
	volMgr *volume.Manager,
	cfg *config.Config,
	logger *logrus.Logger,
) *Manager {
	return &Manager{
		locks:     make(map[string]time.Time),
		repo:      repo,
		volMgr:    volMgr,
		config:    cfg,
		logger:    logger,
		clusterID: cfg.DefaultClusterId,
		podID:     uuid.New().String(), // Unique pod identifier
	}
}

// CreateSnapshotRequest contains parameters for creating a snapshot
type CreateSnapshotRequest struct {
	VolumeID    string
	Name        string
	Description string
	TeamID      string
	UserID      string
}

// CreateSnapshot creates a new snapshot of a volume using JuiceFS COW clone
func (m *Manager) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*db.Snapshot, error) {
	m.logger.WithFields(logrus.Fields{
		"volume_id": req.VolumeID,
		"name":      req.Name,
	}).Info("Creating snapshot")

	// 1. Get volume and verify ownership
	vol, err := m.repo.GetSandboxVolume(ctx, req.VolumeID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrVolumeNotFound
		}
		return nil, fmt.Errorf("get volume: %w", err)
	}

	// Verify team ownership
	if vol.TeamID != req.TeamID {
		return nil, ErrVolumeNotFound // Don't reveal existence
	}

	// 2. Get volume context from volume manager
	volCtx, err := m.volMgr.GetVolume(req.VolumeID)
	if err != nil {
		return nil, fmt.Errorf("get volume context: %w", err)
	}

	// 3. Flush all cached data to ensure consistency
	if err := volCtx.VFS.FlushAll(""); err != nil {
		m.logger.WithError(err).Warn("Failed to flush VFS data before snapshot")
		// Continue anyway - data should still be consistent
	}

	// 4. Look up the volume root directory
	volumePath := fmt.Sprintf("/volumes/%s", req.VolumeID)
	parentIno, rootIno, err := m.lookupPath(volCtx.Meta, volumePath)
	if err != nil {
		return nil, fmt.Errorf("lookup volume path: %w", err)
	}

	// 5. Ensure snapshot parent directory exists
	snapshotID := uuid.New().String()
	snapshotParentPath := fmt.Sprintf("/snapshots/%s", req.VolumeID)

	snapshotParentIno, err := m.ensurePathExists(ctx, volCtx.Meta, snapshotParentPath)
	if err != nil {
		return nil, fmt.Errorf("ensure snapshot parent path: %w", err)
	}

	// 6. Clone volume root to snapshot location using JuiceFS COW
	var cloneCount, cloneTotal uint64
	jfsCtx := meta.Background()

	errno := volCtx.Meta.Clone(jfsCtx, parentIno, rootIno, snapshotParentIno, snapshotID, 0, 0, &cloneCount, &cloneTotal)
	if errno != 0 {
		return nil, fmt.Errorf("%w: %s", ErrCloneFailed, errno.Error())
	}

	m.logger.WithFields(logrus.Fields{
		"volume_id":   req.VolumeID,
		"snapshot_id": snapshotID,
		"clone_count": cloneCount,
		"clone_total": cloneTotal,
	}).Info("JuiceFS clone completed")

	// 7. Look up the new snapshot inode
	snapshotPath := fmt.Sprintf("/snapshots/%s/%s", req.VolumeID, snapshotID)
	_, snapshotIno, err := m.lookupPath(volCtx.Meta, snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("lookup snapshot path: %w", err)
	}

	// 8. Save snapshot metadata to database
	snapshot := &db.Snapshot{
		ID:          snapshotID,
		VolumeID:    req.VolumeID,
		TeamID:      req.TeamID,
		UserID:      req.UserID,
		RootInode:   int64(snapshotIno),
		SourceInode: int64(rootIno),
		Name:        req.Name,
		Description: req.Description,
		SizeBytes:   0, // Logical size, can be computed later
		CreatedAt:   time.Now(),
	}

	if err := m.repo.CreateSnapshot(ctx, snapshot); err != nil {
		// Cleanup: delete the cloned snapshot directory
		m.logger.WithError(err).Error("Failed to save snapshot metadata, cleaning up")
		m.deleteSnapshotDir(ctx, volCtx, snapshotPath)
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"volume_id":   req.VolumeID,
		"snapshot_id": snapshotID,
		"name":        req.Name,
	}).Info("Snapshot created successfully")

	return snapshot, nil
}

// CreateSnapshotSimple is a simplified version for use by HTTP handlers
func (m *Manager) CreateSnapshotSimple(ctx context.Context, req *CreateSnapshotRequest) (*db.Snapshot, error) {
	return m.CreateSnapshot(ctx, req)
}

// ListSnapshots returns all snapshots for a volume
func (m *Manager) ListSnapshots(ctx context.Context, volumeID, teamID string) ([]*db.Snapshot, error) {
	// Verify volume ownership
	vol, err := m.repo.GetSandboxVolume(ctx, volumeID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrVolumeNotFound
		}
		return nil, fmt.Errorf("get volume: %w", err)
	}

	if vol.TeamID != teamID {
		return nil, ErrVolumeNotFound
	}

	return m.repo.ListSnapshotsByVolume(ctx, volumeID)
}

// GetSnapshot retrieves a specific snapshot
func (m *Manager) GetSnapshot(ctx context.Context, volumeID, snapshotID, teamID string) (*db.Snapshot, error) {
	snapshot, err := m.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("get snapshot: %w", err)
	}

	// Verify ownership
	if snapshot.VolumeID != volumeID || snapshot.TeamID != teamID {
		return nil, ErrSnapshotNotFound
	}

	return snapshot, nil
}

// RestoreSnapshotRequest contains parameters for restoring a snapshot
type RestoreSnapshotRequest struct {
	VolumeID   string
	SnapshotID string
	TeamID     string
	UserID     string
}

// RestoreSnapshot restores a volume to a previous snapshot state
func (m *Manager) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) error {
	m.logger.WithFields(logrus.Fields{
		"volume_id":   req.VolumeID,
		"snapshot_id": req.SnapshotID,
	}).Info("Restoring snapshot")

	// 1. Get snapshot and verify ownership
	snapshot, err := m.repo.GetSnapshot(ctx, req.SnapshotID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrSnapshotNotFound
		}
		return fmt.Errorf("get snapshot: %w", err)
	}

	if snapshot.VolumeID != req.VolumeID || snapshot.TeamID != req.TeamID {
		return ErrSnapshotNotBelongToVolume
	}

	// 2. Get volume context
	volCtx, err := m.volMgr.GetVolume(req.VolumeID)
	if err != nil {
		return fmt.Errorf("get volume context: %w", err)
	}

	// 3. Acquire volume lock
	if !m.acquireVolumeLock(req.VolumeID, 30*time.Second) {
		return ErrVolumeLocked
	}
	defer m.releaseVolumeLock(req.VolumeID)

	// 4. Flush all cached data
	if err := volCtx.VFS.FlushAll(""); err != nil {
		m.logger.WithError(err).Warn("Failed to flush VFS data before restore")
	}

	// 5. Look up paths
	volumePath := fmt.Sprintf("/volumes/%s", req.VolumeID)
	parentIno, rootIno, err := m.lookupPath(volCtx.Meta, volumePath)
	if err != nil {
		return fmt.Errorf("lookup volume path: %w", err)
	}

	jfsCtx := meta.Background()
	_ = rootIno // Will be replaced by snapshot

	// 6. Backup current volume by renaming
	tempName := fmt.Sprintf(".restore_%d", time.Now().UnixNano())
	volumeName := filepath.Base(volumePath)

	var renamedIno meta.Ino
	var renamedAttr meta.Attr
	errno := volCtx.Meta.Rename(jfsCtx, parentIno, volumeName, parentIno, tempName, 0, &renamedIno, &renamedAttr)
	if errno != 0 {
		return fmt.Errorf("backup current volume failed: %s", errno.Error())
	}

	// 7. Clone snapshot to volume location
	var cloneCount, cloneTotal uint64
	snapshotParentIno, snapshotIno, err := m.lookupPath(volCtx.Meta, fmt.Sprintf("/snapshots/%s/%s", req.VolumeID, req.SnapshotID))
	if err != nil {
		// Rollback: restore the backup
		m.logger.WithError(err).Error("Failed to lookup snapshot path, rolling back")
		volCtx.Meta.Rename(jfsCtx, parentIno, tempName, parentIno, volumeName, 0, &renamedIno, &renamedAttr)
		return fmt.Errorf("lookup snapshot path: %w", err)
	}

	errno = volCtx.Meta.Clone(jfsCtx, snapshotParentIno, snapshotIno, parentIno, volumeName, 0, 0, &cloneCount, &cloneTotal)
	if errno != 0 {
		// Rollback: restore the backup
		m.logger.WithError(fmt.Errorf(errno.Error())).Error("Clone failed, rolling back")
		volCtx.Meta.Rename(jfsCtx, parentIno, tempName, parentIno, volumeName, 0, &renamedIno, &renamedAttr)
		return fmt.Errorf("%w: %s", ErrCloneFailed, errno.Error())
	}

	// 8. Delete the backup
	var removeCount uint64
	tempIno, _, _ := m.lookupPath(volCtx.Meta, fmt.Sprintf("/volumes/%s", tempName))
	if tempIno > 0 {
		errno = volCtx.Meta.Remove(jfsCtx, parentIno, tempName, true, 4, &removeCount)
		if errno != 0 {
			m.logger.WithError(fmt.Errorf(errno.Error())).Warn("Failed to cleanup backup directory")
		}
	}

	m.logger.WithFields(logrus.Fields{
		"volume_id":   req.VolumeID,
		"snapshot_id": req.SnapshotID,
		"clone_count": cloneCount,
	}).Info("Snapshot restored successfully")

	return nil
}

// DeleteSnapshot removes a snapshot
func (m *Manager) DeleteSnapshot(ctx context.Context, volumeID, snapshotID, teamID string) error {
	m.logger.WithFields(logrus.Fields{
		"volume_id":   volumeID,
		"snapshot_id": snapshotID,
	}).Info("Deleting snapshot")

	// 1. Get snapshot and verify ownership
	snapshot, err := m.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ErrSnapshotNotFound
		}
		return fmt.Errorf("get snapshot: %w", err)
	}

	if snapshot.VolumeID != volumeID || snapshot.TeamID != teamID {
		return ErrSnapshotNotBelongToVolume
	}

	// 2. Get volume context
	volCtx, err := m.volMgr.GetVolume(volumeID)
	if err != nil {
		// Volume might not be mounted, but we can still delete the DB record
		m.logger.WithError(err).Warn("Volume not mounted, deleting DB record only")
	} else {
		// 3. Delete snapshot directory from JuiceFS
		snapshotPath := fmt.Sprintf("/snapshots/%s/%s", volumeID, snapshotID)
		m.deleteSnapshotDir(ctx, volCtx, snapshotPath)
	}

	// 4. Delete database record
	if err := m.repo.DeleteSnapshot(ctx, snapshotID); err != nil {
		return fmt.Errorf("delete snapshot record: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"volume_id":   volumeID,
		"snapshot_id": snapshotID,
	}).Info("Snapshot deleted successfully")

	return nil
}

// Helper functions

// lookupPath resolves a path to parent inode and target inode
func (m *Manager) lookupPath(metaClient meta.Meta, path string) (parentIno, targetIno meta.Ino, err error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return 0, 0, fmt.Errorf("invalid path: %s", path)
	}

	currentIno := meta.RootInode
	var attr meta.Attr

	jfsCtx := meta.Background()

	for i, part := range parts {
		var nextIno meta.Ino
		errno := metaClient.Lookup(jfsCtx, currentIno, part, &nextIno, &attr, true)
		if errno != 0 {
			if errno == syscall.ENOENT {
				return currentIno, 0, fmt.Errorf("path not found: %s", path)
			}
			return 0, 0, fmt.Errorf("lookup %s: %s", part, errno.Error())
		}

		if i == len(parts)-1 {
			return currentIno, nextIno, nil
		}
		currentIno = nextIno
	}

	return currentIno, currentIno, nil
}

// ensurePathExists creates directories along a path if they don't exist
func (m *Manager) ensurePathExists(ctx context.Context, metaClient meta.Meta, path string) (meta.Ino, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return meta.RootInode, nil
	}

	currentIno := meta.RootInode
	var attr meta.Attr

	jfsCtx := meta.Background()

	for _, part := range parts {
		var nextIno meta.Ino
		errno := metaClient.Lookup(jfsCtx, currentIno, part, &nextIno, &attr, false)

		if errno == syscall.ENOENT {
			// Create directory
			errno = metaClient.Mkdir(jfsCtx, currentIno, part, 0755, 0, 0, &nextIno, &attr)
			if errno != 0 && errno != syscall.EEXIST {
				return 0, fmt.Errorf("mkdir %s: %s", part, errno.Error())
			}
			// If EEXIST, look it up again
			if errno == syscall.EEXIST {
				errno = metaClient.Lookup(jfsCtx, currentIno, part, &nextIno, &attr, false)
				if errno != 0 {
					return 0, fmt.Errorf("lookup after mkdir %s: %s", part, errno.Error())
				}
			}
		} else if errno != 0 {
			return 0, fmt.Errorf("lookup %s: %s", part, errno.Error())
		}

		currentIno = nextIno
	}

	return currentIno, nil
}

// deleteSnapshotDir removes a snapshot directory from JuiceFS
func (m *Manager) deleteSnapshotDir(ctx context.Context, volCtx *volume.VolumeContext, snapshotPath string) {
	parentIno, snapshotIno, err := m.lookupPath(volCtx.Meta, snapshotPath)
	if err != nil {
		m.logger.WithError(err).Warn("Failed to lookup snapshot path for deletion")
		return
	}

	if snapshotIno == 0 {
		return // Already deleted
	}

	jfsCtx := meta.Background()
	snapshotName := filepath.Base(snapshotPath)

	var removeCount uint64
	errno := volCtx.Meta.Remove(jfsCtx, parentIno, snapshotName, true, 4, &removeCount)
	if errno != 0 && errno != syscall.ENOENT {
		m.logger.WithError(fmt.Errorf(errno.Error())).Warn("Failed to delete snapshot directory")
	}
}

// acquireVolumeLock tries to acquire a lock for restore operations
func (m *Manager) acquireVolumeLock(volumeID string, timeout time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if lockTime, exists := m.locks[volumeID]; exists {
		// Check if lock has expired
		if time.Since(lockTime) < timeout {
			return false
		}
	}

	m.locks[volumeID] = time.Now()
	return true
}

// releaseVolumeLock releases a volume lock
func (m *Manager) releaseVolumeLock(volumeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, volumeID)
}
