package rootfs

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/pkg/naming"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/layer"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/overlay"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/volume"
	"github.com/sirupsen/logrus"
)

var (
	ErrRootfsNotFound    = fmt.Errorf("rootfs not found")
	ErrSnapshotNotFound  = fmt.Errorf("snapshot not found")
	ErrSnapshotExpired   = fmt.Errorf("snapshot expired")
	ErrInvalidSnapshotID = fmt.Errorf("invalid snapshot id")
	ErrRootfsBusy        = fmt.Errorf("rootfs is busy, try again later")
	ErrFlushFailed       = fmt.Errorf("flush failed")
	ErrCloneFailed       = fmt.Errorf("clone operation failed")
	ErrRestoreFailed     = fmt.Errorf("restore operation failed")
	ErrCleanupFailed     = fmt.Errorf("cleanup failed")
)

// rootfsRepository defines the database operations needed by SnapshotService
type rootfsRepository interface {
	WithTx(ctx context.Context, fn func(pgx.Tx) error) error

	// SandboxRootfs operations
	GetSandboxRootfs(ctx context.Context, sandboxID string) (*db.SandboxRootfs, error)
	GetSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) (*db.SandboxRootfs, error)
	GetSandboxRootfsForUpdate(ctx context.Context, tx pgx.Tx, sandboxID string) (*db.SandboxRootfs, error)
	CreateSandboxRootfsTx(ctx context.Context, tx pgx.Tx, rootfs *db.SandboxRootfs) error
	UpdateSandboxRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, sandboxID, snapshotID string) error
	DeleteSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) error

	// RootfsSnapshot operations
	GetRootfsSnapshot(ctx context.Context, id string) (*db.RootfsSnapshot, error)
	GetRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) (*db.RootfsSnapshot, error)
	GetRootfsSnapshotForUpdate(ctx context.Context, tx pgx.Tx, id string) (*db.RootfsSnapshot, error)
	CreateRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot *db.RootfsSnapshot) error
	DeleteRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) error
	ListRootfsSnapshotsBySandbox(ctx context.Context, sandboxID string, limit, offset int) ([]*db.RootfsSnapshot, int, error)

	// BaseLayer operations
	GetBaseLayer(ctx context.Context, id string) (*db.BaseLayer, error)
	CreateBaseLayer(ctx context.Context, layer *db.BaseLayer) error
	UpdateBaseLayerStatus(ctx context.Context, id, status, lastError string) error
	UpdateBaseLayerExtraction(ctx context.Context, id, imageDigest, layerPath string, sizeBytes int64) error
	IncrementBaseLayerRef(ctx context.Context, id string) (int, error)
	DecrementBaseLayerRef(ctx context.Context, id string) (int, error)
}

// overlayManager defines the overlay operations needed by SnapshotService
type overlayManager interface {
	GetOverlay(sandboxID string) (*overlay.OverlayContext, error)
	CreateOverlay(ctx context.Context, cfg *overlay.CreateOverlayConfig) (*overlay.OverlayContext, error)
	DeleteOverlay(ctx context.Context, sandboxID string) error
	FlushOverlay(ctx context.Context, sandboxID string) error
}

// layerManager defines the layer operations needed by SnapshotService
type layerManager interface {
	IncrementRefCount(ctx context.Context, id string) (int, error)
}

// metaClient interface for JuiceFS meta operations
type metaClient interface {
	Lookup(ctx meta.Context, parent meta.Ino, name string, inode *meta.Ino, attr *meta.Attr, checkPerm bool) syscall.Errno
	Mkdir(ctx meta.Context, parent meta.Ino, name string, mode uint16, umask uint16, cmode uint8, inode *meta.Ino, attr *meta.Attr) syscall.Errno
	Readdir(ctx meta.Context, inode meta.Ino, wantAttr uint8, entries *[]*meta.Entry) syscall.Errno
	Remove(ctx meta.Context, parent meta.Ino, name string, recursive bool, threads int, count *uint64) syscall.Errno
	Clone(ctx meta.Context, srcParent, srcIno meta.Ino, dstParent meta.Ino, dstName string, cmode uint8, cumask uint16, count, total *uint64) syscall.Errno
	Rmdir(ctx meta.Context, parent meta.Ino, name string, skipCheck ...bool) syscall.Errno
	Rename(ctx meta.Context, parentSrc meta.Ino, nameSrc string, parentDst meta.Ino, nameDst string, flags uint32, inode *meta.Ino, attr *meta.Attr) syscall.Errno
}

// SnapshotService manages rootfs snapshots
type SnapshotService struct {
	repo       rootfsRepository
	overlayMgr overlayManager
	layerMgr   layerManager
	metaClient metaClient
	pathNav    *PathNavigator
	logger     *logrus.Logger
}

// NewSnapshotService creates a new snapshot service
func NewSnapshotService(
	repo *db.Repository,
	overlayMgr *overlay.Manager,
	layerMgr *layer.Manager,
	metaClient meta.Meta,
	logger *logrus.Logger,
) *SnapshotService {
	return &SnapshotService{
		repo:       repo,
		overlayMgr: overlayMgr,
		layerMgr:   layerMgr,
		metaClient: metaClient,
		pathNav:    NewPathNavigator(metaClient),
		logger:     logger,
	}
}

// CreateSnapshotRequest contains parameters for creating a snapshot
type CreateSnapshotRequest struct {
	SandboxID        string
	TeamID           string
	Name             string
	Description      string
	Metadata         map[string]string
	RetentionSeconds int
}

// CreateSnapshot creates a point-in-time snapshot of a sandbox's rootfs
func (s *SnapshotService) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*db.RootfsSnapshot, error) {
	return s.createSnapshotInternal(ctx, nil, req)
}

// createSnapshotInternal creates a snapshot without acquiring locks.
// This is used internally by RestoreSnapshot when creating backup snapshots.
// If tx is nil, a new transaction will be started.
func (s *SnapshotService) createSnapshotInternal(ctx context.Context, tx pgx.Tx, req *CreateSnapshotRequest) (*db.RootfsSnapshot, error) {
	var snapshot *db.RootfsSnapshot
	var snapshotPath string

	runInTx := func(tx pgx.Tx) error {
		// 1. Get rootfs with FOR UPDATE NOWAIT to prevent concurrent modifications
		rootfs, err := s.repo.GetSandboxRootfsForUpdate(ctx, tx, req.SandboxID)
		if err != nil {
			if err == db.ErrNotFound {
				return ErrRootfsNotFound
			}
			if db.IsLockContentionError(err) {
				return ErrRootfsBusy
			}
			return fmt.Errorf("get sandbox rootfs: %w", err)
		}

		// 2. Flush the overlay to ensure all data is persisted
		if err := s.overlayMgr.FlushOverlay(ctx, req.SandboxID); err != nil {
			return fmt.Errorf("%w: %v", ErrFlushFailed, err)
		}

		// 3. Get the current upper inode
		overlayCtx, err := s.overlayMgr.GetOverlay(req.SandboxID)
		if err != nil {
			return fmt.Errorf("get overlay: %w", err)
		}

		// 4. Create snapshot record
		snapshotID := overlay.GenerateSnapshotID()
		now := time.Now()

		snapshot = &db.RootfsSnapshot{
			ID:            snapshotID,
			SandboxID:     req.SandboxID,
			TeamID:        req.TeamID,
			BaseLayerID:   rootfs.BaseLayerID,
			UpperVolumeID: rootfs.UpperVolumeID,
			RootInode:     int64(overlayCtx.UpperRootInode),
			SourceInode:   int64(overlayCtx.UpperRootInode),
			Name:          req.Name,
			Description:   req.Description,
			SizeBytes:     0, // Will be updated after clone
			CreatedAt:     now,
		}

		// Set expiry if retention is specified
		if req.RetentionSeconds > 0 {
			expiresAt := now.Add(time.Duration(req.RetentionSeconds) * time.Second)
			snapshot.ExpiresAt = &expiresAt
		}

		// Store metadata as JSON
		if len(req.Metadata) > 0 {
			metadataBytes, err := json.Marshal(req.Metadata)
			if err != nil {
				return fmt.Errorf("marshal metadata: %w", err)
			}
			rawMsg := json.RawMessage(metadataBytes)
			snapshot.Metadata = &rawMsg
		}

		// 5. Create a JuiceFS clone for the snapshot
		snapshotPath, err = naming.JuiceFSRootfsSnapshotPath(req.SandboxID, snapshotID)
		if err != nil {
			return fmt.Errorf("generate snapshot path: %w", err)
		}

		// Clone the upperdir to snapshot path using JuiceFS clone (COW)
		count, total, err := s.cloneUpperdirWithStats(ctx, rootfs.UpperPath, snapshotPath)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCloneFailed, err)
		}
		// Update snapshot size from clone result
		snapshot.SizeBytes = int64(total)

		s.logger.WithFields(logrus.Fields{
			"src_path":     rootfs.UpperPath,
			"dst_path":     snapshotPath,
			"clone_count":  count,
			"clone_total":  total,
		}).Debug("Cloned upperdir to snapshot path")

		// 6. Save to database
		if err := s.repo.CreateRootfsSnapshotTx(ctx, tx, snapshot); err != nil {
			// Cleanup JuiceFS data on database failure
			s.cleanupSnapshotPath(snapshotPath)
			return fmt.Errorf("create snapshot record: %w", err)
		}

		// 7. Update current snapshot reference
		if err := s.repo.UpdateSandboxRootfsSnapshotTx(ctx, tx, req.SandboxID, snapshotID); err != nil {
			s.logger.WithError(err).Warn("Failed to update current snapshot reference")
		}

		return nil
	}

	var err error
	if tx != nil {
		err = runInTx(tx)
	} else {
		err = s.repo.WithTx(ctx, runInTx)
	}

	if err != nil {
		return nil, err
	}

	s.logger.WithFields(logrus.Fields{
		"snapshot_id":  snapshot.ID,
		"sandbox_id":   req.SandboxID,
		"name":         req.Name,
		"snapshotPath": snapshotPath,
		"size_bytes":   snapshot.SizeBytes,
	}).Info("Rootfs snapshot created")

	return snapshot, nil
}

// cloneUpperdirWithStats clones the upperdir to a snapshot path using JuiceFS clone
// and returns the count and total size of cloned files
func (s *SnapshotService) cloneUpperdirWithStats(_ context.Context, upperPath, snapshotPath string) (count, total uint64, err error) {
	jfsCtx := meta.Background()

	count, total, err = s.pathNav.ClonePath(jfsCtx, upperPath, snapshotPath)
	if err != nil {
		return 0, 0, err
	}

	s.logger.WithFields(logrus.Fields{
		"src_path":     upperPath,
		"dst_path":     snapshotPath,
		"clone_count":  count,
		"clone_total":  total,
	}).Debug("Cloned upperdir to snapshot path")

	return count, total, nil
}

// cleanupSnapshotPath removes a snapshot path from JuiceFS with retry logic
func (s *SnapshotService) cleanupSnapshotPath(snapshotPath string) {
	jfsCtx := meta.Background()

	// Try to remove with retries for transient errors
	maxRetries := 3
	retryDelay := 500 * time.Millisecond

	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			s.logger.WithFields(logrus.Fields{
				"path":    snapshotPath,
				"attempt": attempt + 1,
			}).Debug("Retrying snapshot path cleanup")
			time.Sleep(retryDelay)
			retryDelay *= 2 // Exponential backoff
		}

		if err := s.pathNav.RemovePath(jfsCtx, snapshotPath); err != nil {
			lastErr = err
			s.logger.WithError(err).WithFields(logrus.Fields{
				"path":    snapshotPath,
				"attempt": attempt + 1,
			}).Warn("Failed to cleanup snapshot path")
			continue
		}

		// Success
		return
	}

	// All retries failed, log the final error
	s.logger.WithError(lastErr).WithFields(logrus.Fields{
		"path":       snapshotPath,
		"attempts":   maxRetries,
	}).Error("Failed to cleanup snapshot path after all retries")
}

// RestoreSnapshotRequest contains parameters for restoring a snapshot
type RestoreSnapshotRequest struct {
	SandboxID    string
	TeamID       string
	SnapshotID   string
	CreateBackup bool
	BackupName   string
}

// RestoreSnapshot restores a sandbox's rootfs from a snapshot
func (s *SnapshotService) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) (backupID string, err error) {
	var resultBackupID string

	err = s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		// 1. Get snapshot with lock
		snapshot, err := s.repo.GetRootfsSnapshotForUpdate(ctx, tx, req.SnapshotID)
		if err != nil {
			if err == db.ErrNotFound {
				return ErrSnapshotNotFound
			}
			if db.IsLockContentionError(err) {
				return ErrRootfsBusy
			}
			return fmt.Errorf("get snapshot: %w", err)
		}

		// 2. Verify snapshot belongs to sandbox
		if snapshot.SandboxID != req.SandboxID {
			return ErrInvalidSnapshotID
		}

		// 3. Check if snapshot is expired
		if snapshot.ExpiresAt != nil && snapshot.ExpiresAt.Before(time.Now()) {
			return ErrSnapshotExpired
		}

		// 4. Get rootfs with lock
		rootfs, err := s.repo.GetSandboxRootfsForUpdate(ctx, tx, req.SandboxID)
		if err != nil {
			if err == db.ErrNotFound {
				return ErrRootfsNotFound
			}
			if db.IsLockContentionError(err) {
				return ErrRootfsBusy
			}
			return fmt.Errorf("get sandbox rootfs: %w", err)
		}

		// 5. Optionally create a backup of current state
		// Use createSnapshotInternal to avoid deadlock (we already hold the lock)
		if req.CreateBackup {
			backupSnapshot, err := s.createSnapshotInternal(ctx, tx, &CreateSnapshotRequest{
				SandboxID:   req.SandboxID,
				TeamID:      req.TeamID,
				Name:        req.BackupName,
				Description: "Auto-backup before restore",
			})
			if err != nil {
				s.logger.WithError(err).Warn("Failed to create backup snapshot")
			} else {
				resultBackupID = backupSnapshot.ID
			}
		}

		// 6. Flush current state - MUST succeed before restore
		if err := s.overlayMgr.FlushOverlay(ctx, req.SandboxID); err != nil {
			return fmt.Errorf("%w: %v", ErrFlushFailed, err)
		}

		// 7. Get snapshot path
		snapshotPath, err := naming.JuiceFSRootfsSnapshotPath(req.SandboxID, req.SnapshotID)
		if err != nil {
			return fmt.Errorf("get snapshot path: %w", err)
		}

		// 8. Atomic restore: temp dir + rename pattern
		jfsCtx := meta.Background()

		// 8.1. Clone snapshot to temp directory
		tempPath := rootfs.UpperPath + ".restore_" + req.SnapshotID
		if err := s.copySnapshotTo(jfsCtx, snapshotPath, tempPath); err != nil {
			s.cleanupSnapshotPath(tempPath)
			return fmt.Errorf("%w: clone to temp: %v", ErrRestoreFailed, err)
		}

		// 8.2. Backup current upperdir (for rollback if rename fails)
		backupPath := rootfs.UpperPath + ".backup_" + time.Now().Format("20060102150405")
		if err := s.pathNav.Rename(jfsCtx, rootfs.UpperPath, backupPath); err != nil {
			s.cleanupSnapshotPath(tempPath)
			return fmt.Errorf("%w: backup upperdir: %v", ErrRestoreFailed, err)
		}

		// 8.3. Atomic rename: temp directory -> upperdir
		if err := s.pathNav.Rename(jfsCtx, tempPath, rootfs.UpperPath); err != nil {
			// Rollback: restore the backup
			if rollbackErr := s.pathNav.Rename(jfsCtx, backupPath, rootfs.UpperPath); rollbackErr != nil {
				s.logger.WithError(rollbackErr).Error("Failed to rollback upperdir after restore failure")
			}
			s.cleanupSnapshotPath(tempPath)
			return fmt.Errorf("%w: rename temp to upperdir: %v", ErrRestoreFailed, err)
		}

		// 8.4. Async cleanup of backup (keep for a short time for safety)
		go func() {
			time.Sleep(30 * time.Second) // Keep backup for 30 seconds
			s.cleanupSnapshotPath(backupPath)
		}()

		// 9. Update current snapshot reference
		if err := s.repo.UpdateSandboxRootfsSnapshotTx(ctx, tx, req.SandboxID, req.SnapshotID); err != nil {
			s.logger.WithError(err).Warn("Failed to update current snapshot reference")
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	s.logger.WithFields(logrus.Fields{
		"sandbox_id":  req.SandboxID,
		"snapshot_id": req.SnapshotID,
		"backup_id":   resultBackupID,
	}).Info("Rootfs snapshot restored")

	return resultBackupID, nil
}

// clearDirectory removes all contents of a directory without removing the directory itself
func (s *SnapshotService) clearDirectory(jfsCtx meta.Context, path string) error {
	components := pathutil.SplitPath(path)
	current := meta.RootInode
	var attr meta.Attr

	// Navigate to the directory
	for _, component := range components {
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := s.metaClient.Lookup(jfsCtx, current, component, &next, &attr, false)
		if errno != 0 {
			if errno == syscall.ENOENT {
				return nil // Directory doesn't exist, nothing to clear
			}
			return fmt.Errorf("lookup %s: %w", component, errno)
		}
		current = next
	}

	return s.pathNav.ClearDirectory(jfsCtx, current)
}

// copySnapshotTo copies snapshot content to target path using JuiceFS Clone
func (s *SnapshotService) copySnapshotTo(jfsCtx meta.Context, srcPath, dstPath string) error {
	count, total, err := s.pathNav.ClonePath(jfsCtx, srcPath, dstPath)
	if err != nil {
		return err
	}

	s.logger.WithFields(logrus.Fields{
		"src_path":    srcPath,
		"dst_path":    dstPath,
		"clone_count": count,
		"clone_total": total,
	}).Debug("Copied snapshot to target path")

	return nil
}

// ForkRequest contains parameters for forking a sandbox
type ForkRequest struct {
	SourceSandboxID string
	TargetSandboxID string
	TeamID          string
	VolumeConfig    map[string]any
}

// Fork creates a new sandbox rootfs by forking from an existing one
func (s *SnapshotService) Fork(ctx context.Context, req *ForkRequest) (*db.SandboxRootfs, error) {
	var targetRootfs *db.SandboxRootfs

	err := s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		// 1. Get source rootfs with lock
		sourceRootfs, err := s.repo.GetSandboxRootfsForUpdate(ctx, tx, req.SourceSandboxID)
		if err != nil {
			if err == db.ErrNotFound {
				return ErrRootfsNotFound
			}
			if db.IsLockContentionError(err) {
				return ErrRootfsBusy
			}
			return fmt.Errorf("get source rootfs: %w", err)
		}

		// 2. Flush source to ensure consistent state - MUST succeed before fork
		if err := s.overlayMgr.FlushOverlay(ctx, req.SourceSandboxID); err != nil {
			return fmt.Errorf("%w: %v", ErrFlushFailed, err)
		}

		// 3. Build VolumeConfig from request
		volConfig := &volume.VolumeConfig{
			CacheSize:  "1G",
			Prefetch:   0,
			BufferSize: "32M",
			Writeback:  true,
		}
		if req.VolumeConfig != nil {
			if cacheSize, ok := req.VolumeConfig["cache_size"].(string); ok {
				volConfig.CacheSize = cacheSize
			}
			if prefetch, ok := req.VolumeConfig["prefetch"].(int); ok {
				volConfig.Prefetch = prefetch
			}
			if bufferSize, ok := req.VolumeConfig["buffer_size"].(string); ok {
				volConfig.BufferSize = bufferSize
			}
			if writeback, ok := req.VolumeConfig["writeback"].(bool); ok {
				volConfig.Writeback = writeback
			}
		}

		// 4. Create overlay for target sandbox
		overlayCfg := &overlay.CreateOverlayConfig{
			SandboxID:    req.TargetSandboxID,
			TeamID:       req.TeamID,
			BaseLayerID:  sourceRootfs.BaseLayerID,
			VolumeConfig: volConfig,
		}

		// 5. Create the overlay (this creates an empty upperdir)
		_, err = s.overlayMgr.CreateOverlay(ctx, overlayCfg)
		if err != nil {
			return fmt.Errorf("create target overlay: %w", err)
		}

		// 6. Get the created target rootfs to obtain the correct UpperPath
		targetRootfs, err = s.repo.GetSandboxRootfsTx(ctx, tx, req.TargetSandboxID)
		if err != nil {
			// Cleanup on failure
			s.overlayMgr.DeleteOverlay(ctx, req.TargetSandboxID)
			return fmt.Errorf("get target rootfs: %w", err)
		}

		// 7. Copy source upperdir content to target upperdir using JuiceFS Clone
		jfsCtx := meta.Background()

		// First, clear the target upperdir (created by CreateOverlay)
		if err := s.clearDirectory(jfsCtx, targetRootfs.UpperPath); err != nil {
			s.logger.WithError(err).Warn("Failed to clear target upperdir before fork copy")
		}

		// Clone source upperdir to target upperdir
		if err := s.copySnapshotTo(jfsCtx, sourceRootfs.UpperPath, targetRootfs.UpperPath); err != nil {
			// Cleanup on failure
			s.overlayMgr.DeleteOverlay(ctx, req.TargetSandboxID)
			return fmt.Errorf("copy source upperdir: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.WithFields(logrus.Fields{
		"source_sandbox_id": req.SourceSandboxID,
		"target_sandbox_id": req.TargetSandboxID,
	}).Info("Rootfs forked successfully")

	return targetRootfs, nil
}

// ListSnapshots lists all snapshots for a sandbox
func (s *SnapshotService) ListSnapshots(ctx context.Context, sandboxID string, limit, offset int) ([]*db.RootfsSnapshot, int, error) {
	return s.repo.ListRootfsSnapshotsBySandbox(ctx, sandboxID, limit, offset)
}

// GetSnapshot retrieves a specific snapshot
func (s *SnapshotService) GetSnapshot(ctx context.Context, snapshotID string) (*db.RootfsSnapshot, error) {
	snapshot, err := s.repo.GetRootfsSnapshot(ctx, snapshotID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrSnapshotNotFound
		}
		return nil, fmt.Errorf("get snapshot: %w", err)
	}
	return snapshot, nil
}

// DeleteSnapshot deletes a snapshot
func (s *SnapshotService) DeleteSnapshot(ctx context.Context, sandboxID, snapshotID string) error {
	return s.repo.WithTx(ctx, func(tx pgx.Tx) error {
		// 1. Get snapshot with lock
		snapshot, err := s.repo.GetRootfsSnapshotForUpdate(ctx, tx, snapshotID)
		if err != nil {
			if err == db.ErrNotFound {
				return nil // Already deleted
			}
			if db.IsLockContentionError(err) {
				return ErrRootfsBusy
			}
			return fmt.Errorf("get snapshot: %w", err)
		}

		// 2. Verify ownership
		if snapshot.SandboxID != sandboxID {
			return ErrInvalidSnapshotID
		}

		// 3. Delete snapshot data from JuiceFS
		snapshotPath, err := naming.JuiceFSRootfsSnapshotPath(sandboxID, snapshotID)
		if err != nil {
			return fmt.Errorf("get snapshot path: %w", err)
		}

		jfsCtx := meta.Background()
		if err := s.pathNav.RemovePath(jfsCtx, snapshotPath); err != nil {
			s.logger.WithError(err).WithField("path", snapshotPath).Warn("Failed to remove snapshot path from JuiceFS")
			// Continue with database deletion even if JuiceFS removal fails
		}

		// 4. Delete from database
		if err := s.repo.DeleteRootfsSnapshotTx(ctx, tx, snapshotID); err != nil {
			return fmt.Errorf("delete snapshot record: %w", err)
		}

		s.logger.WithFields(logrus.Fields{
			"snapshot_id": snapshotID,
			"sandbox_id":  sandboxID,
		}).Info("Rootfs snapshot deleted")

		return nil
	})
}

// SaveAsLayerRequest contains parameters for saving rootfs as a base layer
type SaveAsLayerRequest struct {
	SandboxID   string
	TeamID      string
	LayerName   string
	Description string
	SnapshotID  string // Optional: save from specific snapshot
}

// SaveAsLayer saves the current rootfs state as a new base layer
func (s *SnapshotService) SaveAsLayer(ctx context.Context, req *SaveAsLayerRequest) (*db.BaseLayer, error) {
	// 1. Get rootfs info
	rootfs, err := s.repo.GetSandboxRootfs(ctx, req.SandboxID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrRootfsNotFound
		}
		return nil, fmt.Errorf("get sandbox rootfs: %w", err)
	}

	// 2. Flush overlay - MUST succeed before save as layer
	if err := s.overlayMgr.FlushOverlay(ctx, req.SandboxID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFlushFailed, err)
	}

	// 3. Create new base layer record
	layerID := req.LayerName
	if layerID == "" {
		layerID = uuid.New().String()
	}

	now := time.Now()
	newLayer := &db.BaseLayer{
		ID:        layerID,
		TeamID:    req.TeamID,
		ImageRef:  fmt.Sprintf("sandbox0/custom/%s", layerID),
		Status:    db.BaseLayerStatusExtracting,
		RefCount:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateBaseLayer(ctx, newLayer); err != nil {
		return nil, fmt.Errorf("create base layer record: %w", err)
	}

	// 4. Create layer directory
	layerPath, err := naming.JuiceFSBaseLayerPath(req.TeamID, layerID)
	if err != nil {
		s.repo.UpdateBaseLayerStatus(ctx, layerID, db.BaseLayerStatusFailed, err.Error())
		return nil, fmt.Errorf("generate layer path: %w", err)
	}

	// 5. Clone the upperdir to the layer path
	jfsCtx := meta.Background()

	// Get source path (either snapshot or current upperdir)
	var srcPath string
	if req.SnapshotID != "" {
		srcPath, _ = naming.JuiceFSRootfsSnapshotPath(req.SandboxID, req.SnapshotID)
	} else {
		srcPath = rootfs.UpperPath
	}

	// Clone to new layer path
	if _, _, err := s.pathNav.ClonePath(jfsCtx, srcPath, layerPath); err != nil {
		s.repo.UpdateBaseLayerStatus(ctx, layerID, db.BaseLayerStatusFailed, err.Error())
		return nil, fmt.Errorf("clone to layer path: %w", err)
	}

	// 6. Update layer status to ready
	if err := s.repo.UpdateBaseLayerExtraction(ctx, layerID, "", layerPath, 0); err != nil {
		s.logger.WithError(err).Warn("Failed to update layer extraction info")
	}

	s.logger.WithFields(logrus.Fields{
		"layer_id":   layerID,
		"sandbox_id": req.SandboxID,
	}).Info("Rootfs saved as base layer")

	return s.repo.GetBaseLayer(ctx, layerID)
}
