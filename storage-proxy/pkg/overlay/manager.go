package overlay

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/pkg/naming"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/volume"
	"github.com/sirupsen/logrus"
)

var (
	ErrOverlayNotFound  = fmt.Errorf("overlay not found")
	ErrLayerNotReady    = fmt.Errorf("base layer not ready")
	ErrVolumeNotMounted = fmt.Errorf("volume not mounted")
)

// OverlayManager defines the interface for overlay filesystem operations
type OverlayManager interface {
	GetOverlay(sandboxID string) (*OverlayContext, error)
	CreateOverlay(ctx context.Context, cfg *CreateOverlayConfig) (*OverlayContext, error)
	DeleteOverlay(ctx context.Context, sandboxID string) error
	FlushOverlay(ctx context.Context, sandboxID string) error
	PrepareMountInfo(ctx context.Context, sandboxID string) (*MountInfo, error)
}

// Manager manages overlay filesystem operations for sandbox rootfs
type Manager struct {
	repo       *db.Repository
	volumeMgr  *volume.Manager
	metaClient meta.Meta
	logger     *logrus.Logger

	mu       sync.RWMutex
	overlays map[string]*OverlayContext // sandboxID -> OverlayContext
}

// OverlayContext holds the context for a sandbox's rootfs overlay
type OverlayContext struct {
	SandboxID      string
	TeamID         string
	BaseLayerID    string
	UpperVolumeID  string
	LowerPath      string // Base layer path (read-only)
	UpperPath      string // Writable layer path
	WorkPath       string // Overlay work directory
	Mounted        bool
	UpperRootInode meta.Ino
}

// NewManager creates a new overlay manager
func NewManager(repo *db.Repository, volumeMgr *volume.Manager, metaClient meta.Meta, logger *logrus.Logger) *Manager {
	return &Manager{
		repo:       repo,
		volumeMgr:  volumeMgr,
		metaClient: metaClient,
		logger:     logger,
		overlays:   make(map[string]*OverlayContext),
	}
}

// CreateOverlayConfig contains configuration for creating an overlay
type CreateOverlayConfig struct {
	SandboxID    string
	TeamID       string
	BaseLayerID  string
	VolumeConfig *volume.VolumeConfig
	FromSnapshot string // Optional: restore from this snapshot
}

// CreateOverlay creates a new overlay for a sandbox
func (m *Manager) CreateOverlay(ctx context.Context, cfg *CreateOverlayConfig) (*OverlayContext, error) {
	// 1. Verify base layer exists and is ready
	layer, err := m.repo.GetBaseLayer(ctx, cfg.BaseLayerID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrLayerNotReady
		}
		return nil, fmt.Errorf("get base layer: %w", err)
	}
	if layer.Status != db.BaseLayerStatusReady {
		return nil, fmt.Errorf("base layer status %s: %w", layer.Status, ErrLayerNotReady)
	}

	// 2. Create paths for the overlay
	upperPath, err := naming.JuiceFSRootfsUpperPath(cfg.SandboxID)
	if err != nil {
		return nil, fmt.Errorf("generate upper path: %w", err)
	}
	workPath, err := naming.JuiceFSRootfsWorkPath(cfg.SandboxID)
	if err != nil {
		return nil, fmt.Errorf("generate work path: %w", err)
	}

	// 3. Create a volume for the upper layer
	upperVolumeID := "rootfs-upper-" + cfg.SandboxID
	volConfig := cfg.VolumeConfig
	if volConfig == nil {
		volConfig = &volume.VolumeConfig{
			CacheSize:  "1G",
			Prefetch:   0,
			BufferSize: "32M",
			Writeback:  true,
		}
	}

	// Mount the upper volume
	s3Prefix := fmt.Sprintf("rootfs/%s", cfg.SandboxID)
	_, _, err = m.volumeMgr.MountVolume(ctx, s3Prefix, upperVolumeID, volConfig, volume.AccessModeRWO)
	if err != nil {
		return nil, fmt.Errorf("mount upper volume: %w", err)
	}

	// 4. Create directory structure in JuiceFS
	jfsCtx := meta.Background()
	upperRootInode, err := m.ensureOverlayDirs(jfsCtx, upperPath, workPath)
	if err != nil {
		m.volumeMgr.UnmountVolume(ctx, upperVolumeID, "")
		return nil, fmt.Errorf("create overlay directories: %w", err)
	}

	// 5. If restoring from snapshot, copy snapshot data to upperdir
	if cfg.FromSnapshot != "" {
		if err := m.restoreFromSnapshot(ctx, cfg.SandboxID, cfg.FromSnapshot, upperPath); err != nil {
			m.volumeMgr.UnmountVolume(ctx, upperVolumeID, "")
			return nil, fmt.Errorf("restore from snapshot: %w", err)
		}
	}

	// 6. Create database record
	rootfs := &db.SandboxRootfs{
		SandboxID:     cfg.SandboxID,
		TeamID:        cfg.TeamID,
		BaseLayerID:   cfg.BaseLayerID,
		UpperVolumeID: upperVolumeID,
		UpperPath:     upperPath,
		WorkPath:      workPath,
	}
	if err := m.repo.CreateSandboxRootfs(ctx, rootfs); err != nil {
		m.volumeMgr.UnmountVolume(ctx, upperVolumeID, "")
		return nil, fmt.Errorf("create sandbox rootfs record: %w", err)
	}

	// 7. Increment base layer ref count
	if _, err := m.repo.IncrementBaseLayerRef(ctx, cfg.BaseLayerID); err != nil {
		m.logger.WithError(err).Warn("Failed to increment base layer ref count")
	}

	// 8. Store in memory
	overlayCtx := &OverlayContext{
		SandboxID:      cfg.SandboxID,
		TeamID:         cfg.TeamID,
		BaseLayerID:    cfg.BaseLayerID,
		UpperVolumeID:  upperVolumeID,
		LowerPath:      layer.LayerPath,
		UpperPath:      upperPath,
		WorkPath:       workPath,
		UpperRootInode: upperRootInode,
	}

	m.mu.Lock()
	m.overlays[cfg.SandboxID] = overlayCtx
	m.mu.Unlock()

	m.logger.WithFields(logrus.Fields{
		"sandbox_id":      cfg.SandboxID,
		"base_layer_id":   cfg.BaseLayerID,
		"upper_volume_id": upperVolumeID,
	}).Info("Overlay created successfully")

	return overlayCtx, nil
}

// ensureOverlayDirs creates the upper and work directories
func (m *Manager) ensureOverlayDirs(jfsCtx meta.Context, upperPath, workPath string) (meta.Ino, error) {
	// Create upper directory
	upperInode, err := m.createDirPath(jfsCtx, upperPath)
	if err != nil {
		return 0, fmt.Errorf("create upper dir: %w", err)
	}

	// Create work directory
	if _, err := m.createDirPath(jfsCtx, workPath); err != nil {
		return 0, fmt.Errorf("create work dir: %w", err)
	}

	return upperInode, nil
}

// createDirPath creates a directory path in JuiceFS
func (m *Manager) createDirPath(jfsCtx meta.Context, path string) (meta.Ino, error) {
	components := pathutil.SplitPath(path)
	current := meta.RootInode
	var attr meta.Attr

	for _, component := range components {
		if component == "" {
			continue
		}

		var next meta.Ino
		errno := m.metaClient.Lookup(jfsCtx, current, component, &next, &attr, false)
		if errno != 0 {
			errno = m.metaClient.Mkdir(jfsCtx, current, component, 0755, 0, 0, &next, &attr)
			if errno != 0 {
				return 0, fmt.Errorf("mkdir %s: %s", component, errno.Error())
			}
		}
		current = next
	}

	return current, nil
}

// restoreFromSnapshot copies snapshot data to the upper directory using JuiceFS Clone
func (m *Manager) restoreFromSnapshot(ctx context.Context, sandboxID, snapshotID, upperPath string) error {
	// Get snapshot info
	snapshot, err := m.repo.GetRootfsSnapshot(ctx, snapshotID)
	if err != nil {
		return fmt.Errorf("get snapshot: %w", err)
	}

	// The snapshot is stored as a JuiceFS clone
	// We need to copy the snapshot content to the upper directory
	snapshotPath, err := naming.JuiceFSRootfsSnapshotPath(sandboxID, snapshotID)
	if err != nil {
		return fmt.Errorf("get snapshot path: %w", err)
	}

	jfsCtx := meta.Background()
	var attr meta.Attr

	// Navigate to snapshot directory and get srcParentIno
	components := pathutil.SplitPath(snapshotPath)
	if len(components) == 0 {
		return fmt.Errorf("invalid snapshot path")
	}

	srcParentIno := meta.RootInode
	srcInode := meta.RootInode
	for _, component := range components {
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := m.metaClient.Lookup(jfsCtx, srcInode, component, &next, &attr, false)
		if errno != 0 {
			return fmt.Errorf("snapshot not found at %s", snapshotPath)
		}
		srcInode = next
	}

	// Find source parent inode (one level up from snapshot)
	srcParentIno = meta.RootInode
	for i := 0; i < len(components)-1; i++ {
		component := components[i]
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := m.metaClient.Lookup(jfsCtx, srcParentIno, component, &next, &attr, false)
		if errno != 0 {
			return fmt.Errorf("snapshot parent path not found")
		}
		srcParentIno = next
	}

	// Navigate to upper directory parent
	upperComponents := pathutil.SplitPath(upperPath)
	if len(upperComponents) == 0 {
		return fmt.Errorf("invalid upper path")
	}

	// First, clear the existing upper directory content
	upperInode := meta.RootInode
	for _, component := range upperComponents {
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := m.metaClient.Lookup(jfsCtx, upperInode, component, &next, &attr, false)
		if errno != 0 {
			// Upper directory doesn't exist yet, which is fine
			break
		}
		upperInode = next
	}

	// Clear the upper directory contents
	if upperInode != meta.RootInode {
		var entries []*meta.Entry
		errno := m.metaClient.Readdir(jfsCtx, upperInode, 0, &entries)
		if errno == 0 {
			for _, entry := range entries {
				name := string(entry.Name)
				if name == "." || name == ".." {
					continue
				}
				var removeCount uint64
				m.metaClient.Remove(jfsCtx, upperInode, name, true, 4, &removeCount)
			}
		}
	}

	// Get upper directory parent for the clone destination
	dstParentIno := meta.RootInode
	for i := 0; i < len(upperComponents)-1; i++ {
		component := upperComponents[i]
		if component == "" {
			continue
		}
		var next meta.Ino
		errno := m.metaClient.Lookup(jfsCtx, dstParentIno, component, &next, &attr, false)
		if errno != 0 {
			return fmt.Errorf("upper parent not found")
		}
		dstParentIno = next
	}

	upperName := upperComponents[len(upperComponents)-1]

	// Remove the existing upper directory first
	var removeCount uint64
	m.metaClient.Remove(jfsCtx, dstParentIno, upperName, true, 4, &removeCount)

	// Clone the snapshot to the upper directory using JuiceFS Clone API
	// Clone signature: Clone(ctx, srcParentIno, srcIno, dstParentIno, dstName, cmode, cumask, count, total)
	var count, total uint64
	errno := m.metaClient.Clone(jfsCtx, srcParentIno, srcInode, dstParentIno, upperName, 0, 0, &count, &total)
	if errno != 0 {
		return fmt.Errorf("clone snapshot to upper: %s", errno.Error())
	}

	m.logger.WithFields(logrus.Fields{
		"snapshot_id":   snapshotID,
		"snapshot_path": snapshotPath,
		"upper_path":    upperPath,
		"source_inode":  srcInode,
		"cloned_count":  count,
		"cloned_total":  total,
		"snapshot_size": snapshot.SizeBytes,
	}).Info("Restored from snapshot successfully")

	return nil
}

// GetOverlay retrieves overlay context for a sandbox
func (m *Manager) GetOverlay(sandboxID string) (*OverlayContext, error) {
	m.mu.RLock()
	overlay, exists := m.overlays[sandboxID]
	m.mu.RUnlock()

	if exists {
		return overlay, nil
	}

	// Try to load from database
	rootfs, err := m.repo.GetSandboxRootfs(context.Background(), sandboxID)
	if err != nil {
		if err == db.ErrNotFound {
			return nil, ErrOverlayNotFound
		}
		return nil, fmt.Errorf("get sandbox rootfs: %w", err)
	}

	// Get base layer
	layer, err := m.repo.GetBaseLayer(context.Background(), rootfs.BaseLayerID)
	if err != nil {
		return nil, fmt.Errorf("get base layer: %w", err)
	}

	overlay = &OverlayContext{
		SandboxID:     sandboxID,
		TeamID:        rootfs.TeamID,
		BaseLayerID:   rootfs.BaseLayerID,
		UpperVolumeID: rootfs.UpperVolumeID,
		LowerPath:     layer.LayerPath,
		UpperPath:     rootfs.UpperPath,
		WorkPath:      rootfs.WorkPath,
	}

	m.mu.Lock()
	m.overlays[sandboxID] = overlay
	m.mu.Unlock()

	return overlay, nil
}

// DeleteOverlay removes an overlay for a sandbox
func (m *Manager) DeleteOverlay(ctx context.Context, sandboxID string) error {
	m.mu.Lock()
	overlay, exists := m.overlays[sandboxID]
	if exists {
		delete(m.overlays, sandboxID)
	}
	m.mu.Unlock()

	if !exists {
		// Try to load from database
		rootfs, err := m.repo.GetSandboxRootfs(ctx, sandboxID)
		if err != nil {
			if err == db.ErrNotFound {
				return nil // Already deleted
			}
			return fmt.Errorf("get sandbox rootfs: %w", err)
		}
		overlay = &OverlayContext{
			SandboxID:     sandboxID,
			UpperVolumeID: rootfs.UpperVolumeID,
			BaseLayerID:   rootfs.BaseLayerID,
		}
	}

	// Unmount the upper volume
	if overlay.UpperVolumeID != "" {
		if err := m.volumeMgr.UnmountVolume(ctx, overlay.UpperVolumeID, ""); err != nil {
			m.logger.WithError(err).WithField("volume_id", overlay.UpperVolumeID).Warn("Failed to unmount upper volume")
		}
	}

	// Decrement base layer ref count
	if overlay.BaseLayerID != "" {
		if _, err := m.repo.DecrementBaseLayerRef(ctx, overlay.BaseLayerID); err != nil {
			m.logger.WithError(err).WithField("base_layer_id", overlay.BaseLayerID).Warn("Failed to decrement base layer ref count")
		}
	}

	// Delete database record
	if err := m.repo.DeleteSandboxRootfs(ctx, sandboxID); err != nil && err != db.ErrNotFound {
		return fmt.Errorf("delete sandbox rootfs record: %w", err)
	}

	m.logger.WithField("sandbox_id", sandboxID).Info("Overlay deleted")

	return nil
}

// PrepareMountInfo returns the information needed to mount the overlay
func (m *Manager) PrepareMountInfo(ctx context.Context, sandboxID string) (*MountInfo, error) {
	overlay, err := m.GetOverlay(sandboxID)
	if err != nil {
		return nil, err
	}

	// Ensure upper volume is mounted
	volCtx, err := m.volumeMgr.GetVolume(overlay.UpperVolumeID)
	if err != nil {
		// Try to mount
		s3Prefix := fmt.Sprintf("rootfs/%s", sandboxID)
		_, _, err = m.volumeMgr.MountVolume(ctx, s3Prefix, overlay.UpperVolumeID, &volume.VolumeConfig{
			CacheSize:  "1G",
			Prefetch:   0,
			BufferSize: "32M",
			Writeback:  true,
		}, volume.AccessModeRWO)
		if err != nil {
			return nil, fmt.Errorf("mount upper volume: %w", err)
		}
		volCtx, err = m.volumeMgr.GetVolume(overlay.UpperVolumeID)
		if err != nil {
			return nil, err
		}
	}

	return &MountInfo{
		SandboxID:  sandboxID,
		LowerPath:  overlay.LowerPath,
		UpperPath:  overlay.UpperPath,
		WorkPath:   overlay.WorkPath,
		UpperInode: volCtx.RootInode,
	}, nil
}

// MountInfo contains the information needed to mount an overlay
type MountInfo struct {
	SandboxID  string
	LowerPath  string
	UpperPath  string
	WorkPath   string
	UpperInode meta.Ino
}

// FlushOverlay flushes all data for an overlay
func (m *Manager) FlushOverlay(ctx context.Context, sandboxID string) error {
	overlay, err := m.GetOverlay(sandboxID)
	if err != nil {
		return err
	}

	volCtx, err := m.volumeMgr.GetVolume(overlay.UpperVolumeID)
	if err != nil {
		return fmt.Errorf("get volume: %w", err)
	}

	// Flush VFS data
	if volCtx.VFS != nil {
		if err := volCtx.VFS.FlushAll(""); err != nil {
			return fmt.Errorf("flush vfs: %w", err)
		}
	}

	return nil
}

// GenerateSnapshotID generates a unique snapshot ID
func GenerateSnapshotID() string {
	return "rs-" + uuid.New().String()[:8]
}
