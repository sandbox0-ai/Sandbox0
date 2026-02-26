// Package gc provides garbage collection utilities for cleaning up expired resources.
package gc

import (
	"context"
	"sync"
	"time"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/rootfs"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/snapshot"
	"github.com/sirupsen/logrus"
)

// Config holds configuration for the snapshot garbage collector.
type Config struct {
	// Interval is the time between GC runs.
	Interval time.Duration
	// BatchSize is the maximum number of expired snapshots to process per run.
	BatchSize int
	// Enabled controls whether GC is active.
	Enabled bool
}

// DefaultConfig returns the default GC configuration.
func DefaultConfig() Config {
	return Config{
		Interval:  5 * time.Minute,
		BatchSize: 100,
		Enabled:   true,
	}
}

// SnapshotGC handles garbage collection of expired snapshots.
type SnapshotGC struct {
	repo       *db.Repository
	rootfsSvc  *rootfs.SnapshotService
	volumeSvc  *snapshot.Manager
	metaClient meta.Meta
	config     Config
	logger     *logrus.Logger

	stopMu sync.Mutex
	stopCh chan struct{}
}

// NewSnapshotGC creates a new snapshot garbage collector.
func NewSnapshotGC(
	repo *db.Repository,
	rootfsSvc *rootfs.SnapshotService,
	volumeSvc *snapshot.Manager,
	metaClient meta.Meta,
	config Config,
	logger *logrus.Logger,
) *SnapshotGC {
	if config.Interval <= 0 {
		config.Interval = DefaultConfig().Interval
	}
	if config.BatchSize <= 0 {
		config.BatchSize = DefaultConfig().BatchSize
	}

	return &SnapshotGC{
		repo:       repo,
		rootfsSvc:  rootfsSvc,
		volumeSvc:  volumeSvc,
		metaClient: metaClient,
		config:     config,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the garbage collection loop.
// It runs in a background goroutine until Stop is called.
func (gc *SnapshotGC) Start(ctx context.Context) error {
	if !gc.config.Enabled {
		gc.logger.Info("Snapshot GC is disabled")
		return nil
	}

	gc.logger.WithFields(logrus.Fields{
		"interval":   gc.config.Interval,
		"batch_size": gc.config.BatchSize,
	}).Info("Starting snapshot garbage collector")

	go gc.runLoop(ctx)
	return nil
}

// Stop stops the garbage collection loop.
func (gc *SnapshotGC) Stop() {
	gc.stopMu.Lock()
	defer gc.stopMu.Unlock()

	select {
	case <-gc.stopCh:
		// Already closed
	default:
		close(gc.stopCh)
	}

	gc.logger.Info("Snapshot garbage collector stopped")
}

// runLoop is the main GC loop.
func (gc *SnapshotGC) runLoop(ctx context.Context) {
	ticker := time.NewTicker(gc.config.Interval)
	defer ticker.Stop()

	// Run once immediately on start
	gc.runOnce(ctx)

	for {
		select {
		case <-gc.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			gc.runOnce(ctx)
		}
	}
}

// runOnce performs a single GC pass.
func (gc *SnapshotGC) runOnce(ctx context.Context) {
	// Clean up expired rootfs snapshots
	rootfsCount, rootfsErrors := gc.cleanupExpiredRootfsSnapshots(ctx)
	if rootfsCount > 0 || rootfsErrors > 0 {
		gc.logger.WithFields(logrus.Fields{
			"deleted": rootfsCount,
			"errors":  rootfsErrors,
		}).Info("Rootfs snapshot GC pass completed")
	}

	// Clean up expired volume snapshots
	volumeCount, volumeErrors := gc.cleanupExpiredVolumeSnapshots(ctx)
	if volumeCount > 0 || volumeErrors > 0 {
		gc.logger.WithFields(logrus.Fields{
			"deleted": volumeCount,
			"errors":  volumeErrors,
		}).Info("Volume snapshot GC pass completed")
	}
}

// cleanupExpiredRootfsSnapshots deletes expired rootfs snapshots.
func (gc *SnapshotGC) cleanupExpiredRootfsSnapshots(ctx context.Context) (deleted, errors int) {
	if gc.repo == nil {
		return 0, 0
	}

	snapshots, err := gc.repo.ListExpiredRootfsSnapshots(ctx, gc.config.BatchSize)
	if err != nil {
		gc.logger.WithError(err).Error("Failed to list expired rootfs snapshots")
		return 0, 1
	}

	for _, snap := range snapshots {
		if err := gc.deleteRootfsSnapshot(ctx, snap); err != nil {
			gc.logger.WithError(err).WithFields(logrus.Fields{
				"snapshot_id": snap.ID,
				"sandbox_id":  snap.SandboxID,
			}).Error("Failed to delete expired rootfs snapshot")
			errors++
			continue
		}
		deleted++
	}

	return deleted, errors
}

// deleteRootfsSnapshot deletes a single rootfs snapshot and its data.
func (gc *SnapshotGC) deleteRootfsSnapshot(ctx context.Context, snap *db.RootfsSnapshot) error {
	if gc.rootfsSvc == nil {
		return nil // Nothing to delete without service
	}

	// Delete from database first (this also handles the JuiceFS cleanup internally)
	// We use the SnapshotService which handles both DB and JuiceFS cleanup
	err := gc.rootfsSvc.DeleteSnapshot(ctx, snap.SandboxID, snap.ID)
	if err != nil {
		// If the snapshot is not found, it's already deleted - that's fine
		if err == rootfs.ErrSnapshotNotFound || err == db.ErrNotFound {
			return nil
		}
		return err
	}

	gc.logger.WithFields(logrus.Fields{
		"snapshot_id": snap.ID,
		"sandbox_id":  snap.SandboxID,
	}).Debug("Deleted expired rootfs snapshot")

	return nil
}

// cleanupExpiredVolumeSnapshots deletes expired volume snapshots.
func (gc *SnapshotGC) cleanupExpiredVolumeSnapshots(ctx context.Context) (deleted, errors int) {
	if gc.repo == nil {
		return 0, 0
	}

	snapshots, err := gc.repo.ListExpiredVolumeSnapshots(ctx, gc.config.BatchSize)
	if err != nil {
		gc.logger.WithError(err).Error("Failed to list expired volume snapshots")
		return 0, 1
	}

	for _, snap := range snapshots {
		if err := gc.deleteVolumeSnapshot(ctx, snap); err != nil {
			gc.logger.WithError(err).WithFields(logrus.Fields{
				"snapshot_id": snap.ID,
				"volume_id":   snap.VolumeID,
			}).Error("Failed to delete expired volume snapshot")
			errors++
			continue
		}
		deleted++
	}

	return deleted, errors
}

// deleteVolumeSnapshot deletes a single volume snapshot and its data.
func (gc *SnapshotGC) deleteVolumeSnapshot(ctx context.Context, snap *db.Snapshot) error {
	if gc.volumeSvc == nil {
		return nil // Nothing to delete without service
	}

	// Delete using the snapshot manager which handles both DB and JuiceFS cleanup
	// DeleteSnapshot requires volumeID, snapshotID, teamID
	err := gc.volumeSvc.DeleteSnapshot(ctx, snap.VolumeID, snap.ID, snap.TeamID)
	if err != nil {
		// If the snapshot is not found, it's already deleted - that's fine
		if err == snapshot.ErrSnapshotNotFound || err == db.ErrNotFound {
			return nil
		}
		return err
	}

	gc.logger.WithFields(logrus.Fields{
		"snapshot_id": snap.ID,
		"volume_id":   snap.VolumeID,
	}).Debug("Deleted expired volume snapshot")

	return nil
}

// RunOnce performs a single GC pass (for manual triggering or testing).
func (gc *SnapshotGC) RunOnce(ctx context.Context) (rootfsDeleted, rootfsErrors, volumeDeleted, volumeErrors int) {
	rootfsDeleted, rootfsErrors = gc.cleanupExpiredRootfsSnapshots(ctx)
	volumeDeleted, volumeErrors = gc.cleanupExpiredVolumeSnapshots(ctx)
	return rootfsDeleted, rootfsErrors, volumeDeleted, volumeErrors
}
