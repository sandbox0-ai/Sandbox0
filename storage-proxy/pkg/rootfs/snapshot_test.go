package rootfs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/overlay"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/pathutil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo implements rootfsRepository for testing
type fakeRepo struct {
	mu             sync.Mutex
	rootfs         map[string]*db.SandboxRootfs
	snapshots      map[string]*db.RootfsSnapshot
	baseLayers     map[string]*db.BaseLayer
	txCount        int
	lockContention bool // Simulate lock contention when true
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rootfs:     make(map[string]*db.SandboxRootfs),
		snapshots:  make(map[string]*db.RootfsSnapshot),
		baseLayers: make(map[string]*db.BaseLayer),
	}
}

func (f *fakeRepo) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	f.mu.Lock()
	f.txCount++
	f.mu.Unlock()
	// Simulate transaction by calling the function directly
	return fn(nil)
}

func (f *fakeRepo) GetSandboxRootfs(ctx context.Context, sandboxID string) (*db.SandboxRootfs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rootfs[sandboxID]; ok {
		return r, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeRepo) GetSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) (*db.SandboxRootfs, error) {
	return f.GetSandboxRootfs(ctx, sandboxID)
}

func (f *fakeRepo) GetSandboxRootfsForUpdate(ctx context.Context, tx pgx.Tx, sandboxID string) (*db.SandboxRootfs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lockContention {
		// Return PostgreSQL 55P03 error (lock not available) to simulate lock contention
		return nil, &pgconn.PgError{Code: "55P03", Message: "could not obtain lock on row"}
	}
	if r, ok := f.rootfs[sandboxID]; ok {
		return r, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeRepo) CreateSandboxRootfsTx(ctx context.Context, tx pgx.Tx, rootfs *db.SandboxRootfs) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rootfs[rootfs.SandboxID] = rootfs
	return nil
}

func (f *fakeRepo) UpdateSandboxRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, sandboxID, snapshotID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rootfs[sandboxID]; ok {
		r.CurrentSnapshotID = &snapshotID
		return nil
	}
	return db.ErrNotFound
}

func (f *fakeRepo) DeleteSandboxRootfsTx(ctx context.Context, tx pgx.Tx, sandboxID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rootfs, sandboxID)
	return nil
}

func (f *fakeRepo) GetRootfsSnapshot(ctx context.Context, id string) (*db.RootfsSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.snapshots[id]; ok {
		return s, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeRepo) GetRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) (*db.RootfsSnapshot, error) {
	return f.GetRootfsSnapshot(ctx, id)
}

func (f *fakeRepo) GetRootfsSnapshotForUpdate(ctx context.Context, tx pgx.Tx, id string) (*db.RootfsSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lockContention {
		// Return PostgreSQL 55P03 error (lock not available) to simulate lock contention
		return nil, &pgconn.PgError{Code: "55P03", Message: "could not obtain lock on row"}
	}
	if s, ok := f.snapshots[id]; ok {
		return s, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeRepo) CreateRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, snapshot *db.RootfsSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots[snapshot.ID] = snapshot
	return nil
}

func (f *fakeRepo) DeleteRootfsSnapshotTx(ctx context.Context, tx pgx.Tx, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.snapshots, id)
	return nil
}

func (f *fakeRepo) ListRootfsSnapshotsBySandbox(ctx context.Context, sandboxID string, limit, offset int) ([]*db.RootfsSnapshot, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*db.RootfsSnapshot
	for _, s := range f.snapshots {
		if s.SandboxID == sandboxID {
			result = append(result, s)
		}
	}
	return result, len(result), nil
}

func (f *fakeRepo) GetBaseLayer(ctx context.Context, id string) (*db.BaseLayer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.baseLayers[id]; ok {
		return l, nil
	}
	return nil, db.ErrNotFound
}

func (f *fakeRepo) CreateBaseLayer(ctx context.Context, layer *db.BaseLayer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.baseLayers[layer.ID] = layer
	return nil
}

func (f *fakeRepo) UpdateBaseLayerStatus(ctx context.Context, id, status, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.baseLayers[id]; ok {
		l.Status = status
		l.LastError = lastError
	}
	return nil
}

func (f *fakeRepo) UpdateBaseLayerExtraction(ctx context.Context, id, imageDigest, layerPath string, sizeBytes int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.baseLayers[id]; ok {
		l.ImageDigest = &imageDigest
		l.LayerPath = layerPath
		l.SizeBytes = sizeBytes
		l.Status = db.BaseLayerStatusReady
	}
	return nil
}

func (f *fakeRepo) IncrementBaseLayerRef(ctx context.Context, id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.baseLayers[id]; ok {
		l.RefCount++
		return l.RefCount, nil
	}
	return 0, db.ErrNotFound
}

func (f *fakeRepo) DecrementBaseLayerRef(ctx context.Context, id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.baseLayers[id]; ok {
		if l.RefCount > 0 {
			l.RefCount--
		}
		return l.RefCount, nil
	}
	return 0, db.ErrNotFound
}

// fakeOverlayManager implements overlayManager for testing
type fakeOverlayManager struct {
	overlays map[string]*overlay.OverlayContext
}

func newFakeOverlayManager() *fakeOverlayManager {
	return &fakeOverlayManager{
		overlays: make(map[string]*overlay.OverlayContext),
	}
}

func (f *fakeOverlayManager) GetOverlay(sandboxID string) (*overlay.OverlayContext, error) {
	if o, ok := f.overlays[sandboxID]; ok {
		return o, nil
	}
	return nil, overlay.ErrOverlayNotFound
}

func (f *fakeOverlayManager) CreateOverlay(ctx context.Context, cfg *overlay.CreateOverlayConfig) (*overlay.OverlayContext, error) {
	o := &overlay.OverlayContext{
		SandboxID:      cfg.SandboxID,
		TeamID:         cfg.TeamID,
		BaseLayerID:    cfg.BaseLayerID,
		UpperVolumeID:  "rootfs-upper-" + cfg.SandboxID,
		UpperPath:      "/rootfs/" + cfg.SandboxID + "/upper",
		WorkPath:       "/rootfs/" + cfg.SandboxID + "/work",
		UpperRootInode: 100,
	}
	f.overlays[cfg.SandboxID] = o
	return o, nil
}

func (f *fakeOverlayManager) DeleteOverlay(ctx context.Context, sandboxID string) error {
	delete(f.overlays, sandboxID)
	return nil
}

func (f *fakeOverlayManager) FlushOverlay(ctx context.Context, sandboxID string) error {
	return nil
}

func (f *fakeOverlayManager) PrepareMountInfo(ctx context.Context, sandboxID string) (*overlay.MountInfo, error) {
	if o, ok := f.overlays[sandboxID]; ok {
		return &overlay.MountInfo{
			SandboxID:  sandboxID,
			LowerPath:  "/layers/" + o.BaseLayerID,
			UpperPath:  o.UpperPath,
			WorkPath:   o.WorkPath,
			UpperInode: o.UpperRootInode,
		}, nil
	}
	return nil, overlay.ErrOverlayNotFound
}

// fakeMetaClient implements metaClient for testing
type fakeMetaClient struct {
	lookupCalls  int
	mkdirCalls   int
	readdirCalls int
	removeCalls  int
	cloneCalls   int
	rmdirCalls   int
	renameCalls  int
}

func (f *fakeMetaClient) Lookup(ctx meta.Context, parent meta.Ino, name string, inode *meta.Ino, attr *meta.Attr, checkPerm bool) syscall.Errno {
	f.lookupCalls++
	*inode = meta.Ino(len(name) + int(parent))
	return 0
}

func (f *fakeMetaClient) Mkdir(ctx meta.Context, parent meta.Ino, name string, mode uint16, umask uint16, cmode uint8, inode *meta.Ino, attr *meta.Attr) syscall.Errno {
	f.mkdirCalls++
	*inode = meta.Ino(len(name) + int(parent))
	return 0
}

func (f *fakeMetaClient) Readdir(ctx meta.Context, inode meta.Ino, wantAttr uint8, entries *[]*meta.Entry) syscall.Errno {
	f.readdirCalls++
	*entries = []*meta.Entry{}
	return 0
}

func (f *fakeMetaClient) Remove(ctx meta.Context, parent meta.Ino, name string, recursive bool, threads int, count *uint64) syscall.Errno {
	f.removeCalls++
	return 0
}

func (f *fakeMetaClient) Clone(ctx meta.Context, srcParent, srcIno meta.Ino, dstParent meta.Ino, dstName string, cmode uint8, cumask uint16, count, total *uint64) syscall.Errno {
	f.cloneCalls++
	*count = 1
	*total = 1024
	return 0
}

func (f *fakeMetaClient) Rmdir(ctx meta.Context, parent meta.Ino, name string, skipCheck ...bool) syscall.Errno {
	f.rmdirCalls++
	return 0
}

func (f *fakeMetaClient) Rename(ctx meta.Context, parentSrc meta.Ino, nameSrc string, parentDst meta.Ino, nameDst string, flags uint32, inode *meta.Ino, attr *meta.Attr) syscall.Errno {
	f.renameCalls++
	return 0
}

// fakeLayerManager implements layerManager for testing
type fakeLayerManager struct {
	refCounts map[string]int
}

func newFakeLayerManager() *fakeLayerManager {
	return &fakeLayerManager{
		refCounts: make(map[string]int),
	}
}

func (f *fakeLayerManager) IncrementRefCount(ctx context.Context, id string) (int, error) {
	f.refCounts[id]++
	return f.refCounts[id], nil
}

func (f *fakeLayerManager) DecrementRefCount(ctx context.Context, id string) (int, error) {
	if f.refCounts[id] > 0 {
		f.refCounts[id]--
	}
	return f.refCounts[id], nil
}

// Test helper to create a SnapshotService with fake dependencies
func newTestSnapshotService(repo *fakeRepo) *SnapshotService {
	metaClient := &fakeMetaClient{}
	return &SnapshotService{
		repo:       repo,
		overlayMgr: newFakeOverlayManager(),
		layerMgr:   newFakeLayerManager(),
		metaClient: metaClient,
		pathNav:    &PathNavigator{metaClient: metaClient},
		logger:     logrus.New(),
	}
}

func newTestSnapshotServiceWithDeps(repo *fakeRepo, overlayMgr overlayManager, metaClient *fakeMetaClient) *SnapshotService {
	return &SnapshotService{
		repo:       repo,
		overlayMgr: overlayMgr,
		layerMgr:   newFakeLayerManager(),
		metaClient: metaClient,
		pathNav:    &PathNavigator{metaClient: metaClient},
		logger:     logrus.New(),
	}
}

// ========================================
// CreateSnapshot Tests
// ========================================

func TestCreateSnapshot_Success(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	// Setup test data
	sandboxID := "test-sandbox-1"
	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperVolumeID:  "vol-1",
		UpperPath:      "/rootfs/test-sandbox-1/upper",
		UpperRootInode: 100,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:     sandboxID,
		TeamID:        "team-1",
		BaseLayerID:   "layer-1",
		UpperVolumeID: "vol-1",
		UpperPath:     "/rootfs/test-sandbox-1/upper",
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &CreateSnapshotRequest{
		SandboxID:        sandboxID,
		TeamID:           "team-1",
		Name:             "test-snapshot",
		Description:      "Test snapshot",
		RetentionSeconds: 3600,
		Metadata:         map[string]string{"key": "value"},
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, snapshot)
	assert.Equal(t, sandboxID, snapshot.SandboxID)
	assert.Equal(t, "test-snapshot", snapshot.Name)
	assert.NotEmpty(t, snapshot.ID)
	assert.NotNil(t, snapshot.ExpiresAt)
	assert.True(t, metaClient.cloneCalls > 0, "Clone should be called")
}

func TestCreateSnapshot_RootfsNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	req := &CreateSnapshotRequest{
		SandboxID: "non-existent-sandbox",
		TeamID:    "team-1",
		Name:      "test-snapshot",
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	assert.ErrorIs(t, err, ErrRootfsNotFound)
	assert.Nil(t, snapshot)
}

func TestCreateSnapshot_DatabaseLockContention(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-locked"
	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      "/rootfs/test-sandbox-locked/upper",
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   "/rootfs/test-sandbox-locked/upper",
	}

	// Simulate database lock contention
	repo.lockContention = true

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &CreateSnapshotRequest{
		SandboxID: sandboxID,
		TeamID:    "team-1",
		Name:      "test-snapshot",
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	// Should return ErrRootfsBusy due to database lock contention
	assert.ErrorIs(t, err, ErrRootfsBusy)
	assert.Nil(t, snapshot)
}

// ========================================
// DeleteSnapshot Tests
// ========================================

func TestDeleteSnapshot_Success(t *testing.T) {
	repo := newFakeRepo()
	metaClient := &fakeMetaClient{}
	svc := newTestSnapshotServiceWithDeps(repo, newFakeOverlayManager(), metaClient)

	sandboxID := "test-sandbox-1"
	snapshotID := "rs-test-snapshot-1"

	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: sandboxID,
		Name:      "test-snapshot",
		CreatedAt: time.Now(),
	}

	err := svc.DeleteSnapshot(context.Background(), sandboxID, snapshotID)

	require.NoError(t, err)
	assert.True(t, metaClient.removeCalls > 0, "Remove should be called")

	// Verify snapshot is deleted
	_, err = repo.GetRootfsSnapshot(context.Background(), snapshotID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func TestDeleteSnapshot_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	err := svc.DeleteSnapshot(context.Background(), "test-sandbox", "non-existent-snapshot")

	// Should not return error for non-existent snapshot (idempotent)
	require.NoError(t, err)
}

func TestDeleteSnapshot_WrongSandbox(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	snapshotID := "rs-test-snapshot-1"
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: "other-sandbox",
		Name:      "test-snapshot",
	}

	err := svc.DeleteSnapshot(context.Background(), "test-sandbox", snapshotID)

	assert.ErrorIs(t, err, ErrInvalidSnapshotID)
}

// ========================================
// RestoreSnapshot Tests
// ========================================

func TestRestoreSnapshot_Success(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	snapshotID := "rs-test-snapshot-1"
	upperPath := "/rootfs/test-sandbox-1/upper"

	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID: sandboxID,
		UpperPath: upperPath,
	}
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: sandboxID,
		Name:      "test-snapshot",
		CreatedAt: time.Now(),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &RestoreSnapshotRequest{
		SandboxID:  sandboxID,
		SnapshotID: snapshotID,
	}

	backupID, err := svc.RestoreSnapshot(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, backupID) // No backup requested
	assert.True(t, metaClient.cloneCalls > 0, "Clone should be called")
}

func TestRestoreSnapshot_WithBackup(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	snapshotID := "rs-test-snapshot-1"
	upperPath := "/rootfs/test-sandbox-1/upper"

	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: sandboxID,
		Name:      "test-snapshot",
		CreatedAt: time.Now(),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &RestoreSnapshotRequest{
		SandboxID:    sandboxID,
		SnapshotID:   snapshotID,
		CreateBackup: true,
		BackupName:   "auto-backup",
	}

	backupID, err := svc.RestoreSnapshot(context.Background(), req)

	require.NoError(t, err)
	// Backup should now be created successfully since we use createSnapshotInternal
	// which doesn't try to acquire the lock again (fixing the deadlock)
	assert.NotEmpty(t, backupID, "Backup should be created successfully")

	// Verify the backup snapshot exists
	backupSnapshot, err := repo.GetRootfsSnapshot(context.Background(), backupID)
	require.NoError(t, err)
	assert.Equal(t, "auto-backup", backupSnapshot.Name)
	assert.Equal(t, sandboxID, backupSnapshot.SandboxID)
}

func TestRestoreSnapshot_Expired(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	snapshotID := "rs-test-snapshot-1"
	expiredTime := time.Now().Add(-1 * time.Hour)

	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		UpperRootInode: 100,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID: sandboxID,
	}
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: sandboxID,
		Name:      "test-snapshot",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: &expiredTime,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &RestoreSnapshotRequest{
		SandboxID:  sandboxID,
		SnapshotID: snapshotID,
	}

	backupID, err := svc.RestoreSnapshot(context.Background(), req)

	assert.ErrorIs(t, err, ErrSnapshotExpired)
	assert.Empty(t, backupID)
}

// ========================================
// Fork Tests
// ========================================

func TestFork_Success(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sourceSandboxID := "source-sandbox"
	targetSandboxID := "target-sandbox"
	sourceUpperPath := "/rootfs/source/upper"
	targetUpperPath := "/rootfs/target/upper"

	// Setup source sandbox
	repo.rootfs[sourceSandboxID] = &db.SandboxRootfs{
		SandboxID:   sourceSandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   sourceUpperPath,
	}
	overlayMgr.overlays[sourceSandboxID] = &overlay.OverlayContext{
		SandboxID:      sourceSandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      sourceUpperPath,
	}

	// Pre-create target rootfs since fakeOverlayManager.CreateOverlay doesn't create it
	repo.rootfs[targetSandboxID] = &db.SandboxRootfs{
		SandboxID:   targetSandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   targetUpperPath,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &ForkRequest{
		SourceSandboxID: sourceSandboxID,
		TargetSandboxID: targetSandboxID,
		TeamID:          "team-1",
	}

	targetRootfs, err := svc.Fork(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, targetRootfs)
	assert.True(t, metaClient.cloneCalls > 0, "Clone should be called for fork")
}

func TestFork_WithVolumeConfig(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sourceSandboxID := "source-sandbox"
	targetSandboxID := "target-sandbox"
	sourceUpperPath := "/rootfs/source/upper"
	targetUpperPath := "/rootfs/target/upper"

	repo.rootfs[sourceSandboxID] = &db.SandboxRootfs{
		SandboxID:   sourceSandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   sourceUpperPath,
	}
	overlayMgr.overlays[sourceSandboxID] = &overlay.OverlayContext{
		SandboxID:      sourceSandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      sourceUpperPath,
	}

	// Pre-create target rootfs
	repo.rootfs[targetSandboxID] = &db.SandboxRootfs{
		SandboxID:   targetSandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   targetUpperPath,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &ForkRequest{
		SourceSandboxID: sourceSandboxID,
		TargetSandboxID: targetSandboxID,
		TeamID:          "team-1",
		VolumeConfig: map[string]any{
			"cache_size":  "2G",
			"buffer_size": "64M",
			"writeback":   false,
			"prefetch":    3,
		},
	}

	targetRootfs, err := svc.Fork(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, targetRootfs)
}

func TestFork_SourceNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	req := &ForkRequest{
		SourceSandboxID: "non-existent",
		TargetSandboxID: "target",
		TeamID:          "team-1",
	}

	targetRootfs, err := svc.Fork(context.Background(), req)

	assert.ErrorIs(t, err, ErrRootfsNotFound)
	assert.Nil(t, targetRootfs)
}

// ========================================
// GetSnapshot Tests
// ========================================

func TestGetSnapshot_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	snapshotID := "rs-test-snapshot"
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: "sandbox-1",
		Name:      "test-snapshot",
	}

	snapshot, err := svc.GetSnapshot(context.Background(), snapshotID)

	require.NoError(t, err)
	assert.Equal(t, snapshotID, snapshot.ID)
}

func TestGetSnapshot_NotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	snapshot, err := svc.GetSnapshot(context.Background(), "non-existent")

	assert.ErrorIs(t, err, ErrSnapshotNotFound)
	assert.Nil(t, snapshot)
}

// ========================================
// ListSnapshots Tests
// ========================================

func TestListSnapshots(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestSnapshotService(repo)

	sandboxID := "sandbox-1"
	repo.snapshots["rs-1"] = &db.RootfsSnapshot{ID: "rs-1", SandboxID: sandboxID, Name: "snap-1"}
	repo.snapshots["rs-2"] = &db.RootfsSnapshot{ID: "rs-2", SandboxID: sandboxID, Name: "snap-2"}
	repo.snapshots["rs-3"] = &db.RootfsSnapshot{ID: "rs-3", SandboxID: "other-sandbox", Name: "snap-3"}

	snapshots, total, err := svc.ListSnapshots(context.Background(), sandboxID, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, snapshots, 2)
}

// ========================================
// SaveAsLayer Tests
// ========================================

func TestSaveAsLayer_Success(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "sandbox-1"
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   "/rootfs/sandbox-1/upper",
	}
	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		UpperRootInode: 100,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &SaveAsLayerRequest{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		LayerName:   "custom-layer",
		Description: "Custom base layer",
	}

	layer, err := svc.SaveAsLayer(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, layer)
	assert.Equal(t, "custom-layer", layer.ID)
	assert.True(t, metaClient.cloneCalls > 0, "Clone should be called")
}

// ========================================
// PathNavigator Tests
// ========================================

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path     string
		expected []string
	}{
		{"", nil},
		{"/", []string{}}, // Empty slice, not nil
		{"/a", []string{"a"}},
		{"/a/b/c", []string{"a", "b", "c"}},
		{"a/b/c", []string{"a", "b", "c"}},
		{"/a//b///c", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := pathutil.SplitPath(tt.path)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestPathNavigator_NavigatePath(t *testing.T) {
	metaClient := &fakeMetaClient{}
	nav := &PathNavigator{metaClient: metaClient}

	info, err := nav.NavigatePath(meta.Background(), "/rootfs/sandbox/upper")

	require.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, metaClient.lookupCalls > 0)
}

func TestPathNavigator_ClonePath(t *testing.T) {
	metaClient := &fakeMetaClient{}
	nav := &PathNavigator{metaClient: metaClient}

	count, total, err := nav.ClonePath(meta.Background(), "/source", "/dest")

	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
	assert.Equal(t, uint64(1024), total)
	assert.True(t, metaClient.cloneCalls > 0)
}

func TestPathNavigator_RemovePath(t *testing.T) {
	metaClient := &fakeMetaClient{}
	nav := &PathNavigator{metaClient: metaClient}

	err := nav.RemovePath(meta.Background(), "/rootfs/snapshot")

	require.NoError(t, err)
	assert.True(t, metaClient.removeCalls > 0)
}

// ========================================
// Metadata Tests
// ========================================

func TestCreateSnapshot_WithMetadata(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	upperPath := "/rootfs/test-sandbox-1/upper"
	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	metadata := map[string]string{
		"environment": "production",
		"version":     "1.0.0",
	}

	req := &CreateSnapshotRequest{
		SandboxID: sandboxID,
		TeamID:    "team-1",
		Name:      "test-snapshot",
		Metadata:  metadata,
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, snapshot.Metadata)

	// Verify metadata can be unmarshaled
	var unmarshaled map[string]string
	err = json.Unmarshal(*snapshot.Metadata, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, metadata, unmarshaled)
}

// ========================================
// Flush Failure Tests
// ========================================

// fakeOverlayManagerWithFlushError is an overlay manager that simulates flush failures
type fakeOverlayManagerWithFlushError struct {
	*fakeOverlayManager
	flushError error
}

func (f *fakeOverlayManagerWithFlushError) FlushOverlay(ctx context.Context, sandboxID string) error {
	if f.flushError != nil {
		return f.flushError
	}
	return nil
}

func TestCreateSnapshot_FlushFailure(t *testing.T) {
	repo := newFakeRepo()
	baseOverlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	upperPath := "/rootfs/test-sandbox-1/upper"
	baseOverlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}

	// Create overlay manager with flush error
	overlayMgr := &fakeOverlayManagerWithFlushError{
		fakeOverlayManager: baseOverlayMgr,
		flushError:         errors.New("flush failed"),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &CreateSnapshotRequest{
		SandboxID: sandboxID,
		TeamID:    "team-1",
		Name:      "test-snapshot",
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushFailed)
	assert.Nil(t, snapshot)
}

func TestRestoreSnapshot_FlushFailure(t *testing.T) {
	repo := newFakeRepo()
	baseOverlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	snapshotID := "rs-test-snapshot-1"
	upperPath := "/rootfs/test-sandbox-1/upper"

	baseOverlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}
	repo.snapshots[snapshotID] = &db.RootfsSnapshot{
		ID:        snapshotID,
		SandboxID: sandboxID,
		Name:      "test-snapshot",
		CreatedAt: time.Now(),
	}

	// Create overlay manager with flush error
	overlayMgr := &fakeOverlayManagerWithFlushError{
		fakeOverlayManager: baseOverlayMgr,
		flushError:         errors.New("flush failed"),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &RestoreSnapshotRequest{
		SandboxID:  sandboxID,
		SnapshotID: snapshotID,
	}

	backupID, err := svc.RestoreSnapshot(context.Background(), req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushFailed)
	assert.Empty(t, backupID)
}

func TestFork_FlushFailure(t *testing.T) {
	repo := newFakeRepo()
	baseOverlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sourceSandboxID := "source-sandbox"
	sourceUpperPath := "/rootfs/source/upper"

	baseOverlayMgr.overlays[sourceSandboxID] = &overlay.OverlayContext{
		SandboxID:      sourceSandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      sourceUpperPath,
	}
	repo.rootfs[sourceSandboxID] = &db.SandboxRootfs{
		SandboxID:   sourceSandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   sourceUpperPath,
	}

	// Create overlay manager with flush error
	overlayMgr := &fakeOverlayManagerWithFlushError{
		fakeOverlayManager: baseOverlayMgr,
		flushError:         errors.New("flush failed"),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &ForkRequest{
		SourceSandboxID: sourceSandboxID,
		TargetSandboxID: "target-sandbox",
		TeamID:          "team-1",
	}

	targetRootfs, err := svc.Fork(context.Background(), req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushFailed)
	assert.Nil(t, targetRootfs)
}

func TestSaveAsLayer_FlushFailure(t *testing.T) {
	repo := newFakeRepo()
	baseOverlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "sandbox-1"
	upperPath := "/rootfs/sandbox-1/upper"

	baseOverlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}

	// Create overlay manager with flush error
	overlayMgr := &fakeOverlayManagerWithFlushError{
		fakeOverlayManager: baseOverlayMgr,
		flushError:         errors.New("flush failed"),
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &SaveAsLayerRequest{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		LayerName:   "custom-layer",
		Description: "Custom base layer",
	}

	layer, err := svc.SaveAsLayer(context.Background(), req)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushFailed)
	assert.Nil(t, layer)
}

// ========================================
// PathNavigator Rename Tests
// ========================================

func TestPathNavigator_Rename(t *testing.T) {
	metaClient := &fakeMetaClient{}
	nav := &PathNavigator{metaClient: metaClient}

	err := nav.Rename(meta.Background(), "/source", "/dest")

	require.NoError(t, err)
	assert.True(t, metaClient.renameCalls > 0)
}

// ========================================
// Snapshot Size Tests
// ========================================

func TestCreateSnapshot_RecordsSize(t *testing.T) {
	repo := newFakeRepo()
	overlayMgr := newFakeOverlayManager()
	metaClient := &fakeMetaClient{}

	sandboxID := "test-sandbox-1"
	upperPath := "/rootfs/test-sandbox-1/upper"
	overlayMgr.overlays[sandboxID] = &overlay.OverlayContext{
		SandboxID:      sandboxID,
		TeamID:         "team-1",
		BaseLayerID:    "layer-1",
		UpperRootInode: 100,
		UpperPath:      upperPath,
	}
	repo.rootfs[sandboxID] = &db.SandboxRootfs{
		SandboxID:   sandboxID,
		TeamID:      "team-1",
		BaseLayerID: "layer-1",
		UpperPath:   upperPath,
	}

	svc := newTestSnapshotServiceWithDeps(repo, overlayMgr, metaClient)

	req := &CreateSnapshotRequest{
		SandboxID: sandboxID,
		TeamID:    "team-1",
		Name:      "test-snapshot",
	}

	snapshot, err := svc.CreateSnapshot(context.Background(), req)

	require.NoError(t, err)
	assert.NotNil(t, snapshot)
	// Size should be recorded from ClonePath (fakeMetaClient returns 1024)
	assert.Equal(t, int64(1024), snapshot.SizeBytes)
}
