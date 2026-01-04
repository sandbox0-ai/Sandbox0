# Procd - 卷管理设计规范

## 一、设计目标

Sandbox0Volume 是一个独立于 Sandbox 的持久化存储资源，基于 **JuiceFS**（S3 支持的 POSIX 文件系统）构建。主要特性：

1. **高性能冷启动**：直接使用内核 FUSE 挂载 JuiceFS，挂载时间 <100ms
2. **多租户隔离**：每个卷使用 JuiceFS 子目录，并配备访问控制
3. **独立生命周期**：卷可以独立于 Sandbox 创建/删除
4. **灵活挂载**：一个卷可以被多个 sandbox 挂载（第一个获得读写权限，其他为只读）
5. **快速快照**：通过 JuiceFS 或简单的 S3 复制操作实现 S3 目录快照

---

## 二、Core Concepts

### 2.1 Resource Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Sandbox0Volume Resource Model                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Volume (Independent Resource)                                              │
│  ├── ID: vol-abc123                                                        │
│  ├── Name: my-workspace                                                    │
│  ├── TeamID: team-456                                                      │
│  ├── JuiceFS Config:                                                       │
│  │   ├── MetaURL: postgres://postgres:5432/sandbox0?sslmode=disable      │
│  │   ├── S3Bucket: sandbox0-volumes                                       │
│  │   ├── S3Prefix: teams/team-456/volumes/vol-abc123                     │
│  │   └── Subdir: /                                                        │
│  ├── Encryption: AES-256-GCM (client-side)                                │
│  └── Access Control: AllowedSandboxIDs: [sb-1, sb-2]                      │
│                                                                             │
│  JuiceFS Architecture:                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Procd (pid=1)                                                       │  │
│  │  ├── Embedded JuiceFS Library                                        │  │
│  │  │   ├── vfs.VFS - Virtual File System Layer                         │  │
│  │  │   ├── meta.Client - Metadata Engine (Redis/PostgreSQL)            │  │
│  │  │   ├── chunk.CachedStore - Object Storage + Local Cache           │  │
│  │  │   └── fuse.Serve - FUSE Server (mounts to /workspace)             │  │
│  │  └── /dev/fuse - Provided by k8s-plugin (no privileged)         │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  JuiceFS Metadata (PostgreSQL)                                       │  │
│  │  ├── Inodes, Dentries                                                │  │
│  │  ├── File Metadata (size, mtime, permissions)                        │  │
│  │  └── Chunk References (object keys in S3)                            │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  S3 Object Storage (Data Chunks)                                     │  │
│  │  s3://sandbox0-volumes/                                               │  │
│  │  ├── teams/team-456/volumes/vol-abc123/                               │  │
│  │  │   ├── chunks/                                                      │  │
│  │  │   │   ├── 0/0/1_0_4194304   (4MB chunk)                           │  │
│  │  │   │   ├── 0/0/1_1_4194304                                         │  │
│  │  │   │   └── ...                                                     │  │
│  │  │   └── .jfs/...               (JuiceFS metadata backup)            │  │
│  │  └── snapshots/                                                       │  │
│  │      ├── snap-001/               (S3 snapshot copy)                   │  │
│  │      └── snap-002/                                                    │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Volume-Sandbox Relationship

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     Volume ↔ Sandbox Relationship                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Volume (vol-123)                Volume (vol-456)                           │
│       │                                 │                                  │
│       ├──► Sandbox (sb-a) [Read-Write]  ├──► Sandbox (sb-c) [Read-Write]   │
│       ├──► Sandbox (sb-b) [Read-Only]   └──► Sandbox (sb-d) [Read-Only]    │
│       │                                                                     │
│       └──► Independent Existence         └──► Independent Existence        │
│                                                                             │
│  Relationship Rules:                                                        │
│  - 1 Volume can be mounted by multiple Sandboxes                           │
│  - First mounter gets read-write access, subsequent mounters get read-only │
│  - 1 Sandbox can mount multiple Volumes                                    │
│  - Sandbox deletion does not affect Volumes                                │
│  - Volumes can exist without being mounted by any Sandbox                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Storage Architecture (JuiceFS-based)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Pod Architecture (Procd only, no Sidecar)                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Procd Container (PID=1)                                             │   │
│  │  ┌───────────────────────────────────────────────────────────────┐  │   │
│  │  │  JuiceFS Mount (via embedded library)                          │  │   │
│  │  │  ┌─────────────────────────────────────────────────────────┐  │  │   │
│  │  │  │  /workspace (FUSE mount point)                           │  │  │   │
│  │  │  │  ├── project/                                            │  │  │   │
│  │  │  │  ├── data/                                               │  │  │   │
│  │  │  │  └── ...                                                 │  │  │   │
│  │  │  └─────────────────────────────────────────────────────────┘  │  │   │
│  │  │                                                                 │  │   │
│  │  │  Embedded JuiceFS Components:                                  │  │   │
│  │  │  ├── fuse.Serve() - FUSE server (goroutine)                   │  │   │
│  │  │  ├── vfs.VFS - File system operations                          │  │   │
│  │  │  ├── meta.Client - Metadata operations (PostgreSQL client)     │  │   │
│  │  │  ├── chunk.CachedStore - S3 operations + local cache           │  │   │
│  │  │  │   ├── Local Cache: /var/lib/juicefs/cache (10GB)           │  │   │
│  │  │  │   └── S3 Backend: sandbox0-volumes bucket                   │  │   │
│  │  │  └── object.ObjectStorage - S3 client                          │  │   │
│  │  └───────────────────────────────────────────────────────────────┘  │   │
│  │                                                                       │   │
│  │  securityContext:                                                    │   │
│  │    capabilities:                                                     │   │
│  │      add: [NET_ADMIN]  # For nftables only                          │   │
│  │                        # NO SYS_ADMIN needed                         │   │
│  │                                                                       │   │
│  │  volumeMounts:                                                       │   │
│  │    - name: fuse-device                                               │   │
│  │      mountPath: /dev/fuse  # Provided by k8s-plugin            │   │
│  │    - name: cache                                                     │   │
│  │      mountPath: /var/lib/juicefs/cache                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  k8s-plugin (DaemonSet)                                         │   │
│  │  - Exposes /dev/fuse to containers                                   │   │
│  │  - No privileged mode required                                       │   │
│  │  - Pod resource: limits: sandbox0.ai/fuse: 1                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、Data Structure Definitions

### 3.1 Volume

```go
// Volume Sandbox0 persistent volume (stored in PostgreSQL, managed by Manager)
type Volume struct {
    // Basic attributes
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    TeamID      string    `json:"team_id" db:"team_id"`
    Description string    `json:"description,omitempty" db:"description"`

    // JuiceFS configuration
    JuiceFS JuiceFSConfig `json:"juicefs" db:"juicefs"`

    // Capacity
    Capacity    string    `json:"capacity" db:"capacity"`    // e.g. "10Gi"

    // Encryption (client-side, managed by JuiceFS)
    EncryptionKeyID string `json:"encryption_key_id,omitempty" db:"encryption_key_id"`

    // Access control
    ReadOnly          bool     `json:"read_only" db:"read_only"`
    AllowedSandboxIDs []string `json:"allowed_sandbox_ids,omitempty" db:"allowed_sandbox_ids"`

    // Tags and metadata
    Tags        []string          `json:"tags,omitempty" db:"tags"`
    Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`

    // Timestamps
    CreatedAt      time.Time `json:"created_at" db:"created_at"`
    UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
    LastAccessedAt time.Time `json:"last_accessed_at" db:"last_accessed_at"`

    // Statistics
    SizeBytes   int64 `json:"size_bytes" db:"size_bytes"`     // Actual usage
    FileCount   int32 `json:"file_count" db:"file_count"`
    MountCount  int32 `json:"mount_count" db:"mount_count"`   // Currently mounted
}

// JuiceFSConfig JuiceFS mount configuration
type JuiceFSConfig struct {
    // Metadata engine (unified PostgreSQL)
    MetaURL     string `json:"meta_url"`     // e.g. "postgres://postgres:5432/sandbox0?sslmode=disable"
    
    // Object storage
    S3Bucket    string `json:"s3_bucket"`    // e.g. "sandbox0-volumes"
    S3Prefix    string `json:"s3_prefix"`    // e.g. "teams/team-456/volumes/vol-abc123"
    S3Region    string `json:"s3_region"`    // e.g. "us-east-1"
    S3Endpoint  string `json:"s3_endpoint,omitempty"`  // Optional custom endpoint
    
    // Mount options
    CacheDir    string `json:"cache_dir"`    // e.g. "/var/lib/juicefs/cache"
    CacheSize   string `json:"cache_size"`   // e.g. "10Gi"
    ReadOnly    bool   `json:"read_only"`
    Subdir      string `json:"subdir,omitempty"`  // Optional subdir within volume
    
    // Performance tuning
    Prefetch    int    `json:"prefetch,omitempty"`     // Prefetch N chunks
    BufferSize  string `json:"buffer_size,omitempty"`  // e.g. "300Mi"
    Writeback   bool   `json:"writeback,omitempty"`    // Enable writeback cache
}

// VolumeStatus Volume runtime status (not persisted in DB)
type VolumeStatus struct {
    VolumeID    string
    IsMounted   bool
    MountedBy   []string  // List of Sandbox IDs currently mounting this volume
    MountPoint  string    // e.g. "/workspace"
    JuiceFSPID  int       // JuiceFS FUSE server goroutine reference (not OS PID)
}
```

### 3.2 Snapshot

```go
// Snapshot Volume snapshot (points to an S3 directory copy)
type Snapshot struct {
    ID          string    `json:"id" db:"id"`
    VolumeID    string    `json:"volume_id" db:"volume_id"`
    Name        string    `json:"name" db:"name"`
    Description string    `json:"description,omitempty" db:"description"`

    // Snapshot storage location (S3 copy)
    S3Path      string `json:"s3_path" db:"s3_path"`  // e.g. "snapshots/snap-001/"
    
    // Snapshot method
    Method      SnapshotMethod `json:"method" db:"method"`  // "s3_copy" | "juicefs_clone"
    
    // Optional: tags and metadata
    Tags        []string          `json:"tags,omitempty" db:"tags"`
    Metadata    map[string]string `json:"metadata,omitempty" db:"metadata"`

    // Timestamps
    CreatedAt   time.Time  `json:"created_at" db:"created_at"`
    ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`

    // Statistics (snapshot size)
    SizeBytes   int64 `json:"size_bytes" db:"size_bytes"`
    FileCount   int32 `json:"file_count" db:"file_count"`
}

type SnapshotMethod string

const (
    SnapshotMethodS3Copy      SnapshotMethod = "s3_copy"       // Simple S3 directory copy
    SnapshotMethodJuiceFSClone SnapshotMethod = "juicefs_clone" // JuiceFS native clone (if supported)
)

// SnapshotRestoreConfig Snapshot restore configuration
type SnapshotRestoreConfig struct {
    SnapshotID  string `json:"snapshot_id"`
    TargetVolume string `json:"target_volume,omitempty"`  // Optional, create new volume from snapshot
}
```

---

## 四、VolumeManager Implementation

### 4.1 Core Interface

```go
// VolumeManager Volume manager (integrated in Procd)
type VolumeManager struct {
    // JuiceFS configuration
    defaultMetaURL string  // Default Redis/PostgreSQL URL
    s3Config       *S3Config
    
    // Local cache configuration
    cacheRoot   string  // /var/lib/juicefs
    
    // Active mounts
    mounts      sync.Map  // volumeID -> *MountContext
    
    // Configuration
    config *VolumeManagerConfig
}

// MountContext JuiceFS mount context
type MountContext struct {
    VolumeID    string
    MountPoint  string
    
    // JuiceFS components (embedded)
    vfs         *vfs.VFS
    metaClient  meta.Meta
    chunkStore  chunk.ChunkStore
    
    // Shutdown control
    cancel      context.CancelFunc
    done        chan struct{}
    
    // Mount metadata
    ReadOnly    bool
    MountedAt   time.Time
}

type VolumeManagerConfig struct {
    MetaURL         string
    S3Region        string
    S3Endpoint      string
    DefaultCacheSize string  // e.g. "10Gi"
    CacheRoot       string   // /var/lib/juicefs
}

// Mount mounts a volume using embedded JuiceFS library
func (vm *VolumeManager) Mount(ctx context.Context, req *MountRequest) (*MountResponse, error)

// Unmount unmounts a volume
func (vm *VolumeManager) Unmount(ctx context.Context, volumeID string) error

// CreateSnapshot creates a snapshot (S3 copy)
func (vm *VolumeManager) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*Snapshot, error)

// RestoreSnapshot restores from a snapshot
func (vm *VolumeManager) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) error

// ListSnapshots lists all snapshots for a volume
func (vm *VolumeManager) ListSnapshots(ctx context.Context, volumeID string) ([]*Snapshot, error)
```

### 4.2 Mount Implementation (Embedded JuiceFS)

```go
// MountRequest Mount request
type MountRequest struct {
    VolumeID    string            `json:"volume_id"`
    SandboxID   string            `json:"sandbox_id"`
    MountPoint  string            `json:"mount_point"`  // e.g. "/workspace"
    ReadOnly    bool              `json:"read_only,omitempty"`
    SnapshotID  string            `json:"snapshot_id,omitempty"`  // Optional: mount from snapshot
    JuiceFSOpts map[string]string `json:"juicefs_opts,omitempty"` // Optional: JuiceFS mount options
}

// MountResponse Mount response
type MountResponse struct {
    VolumeID       string `json:"volume_id"`
    MountPoint     string `json:"mount_point"`
    JuiceFSVersion string `json:"juicefs_version"`
    CacheHit       bool   `json:"cache_hit"`  // Whether local cache was used
}

// Mount mounts a volume by embedding JuiceFS library
func (vm *VolumeManager) Mount(ctx context.Context, req *MountRequest) (*MountResponse, error) {
    // 1. Load Volume metadata from Manager (or cache)
    volume, err := vm.loadVolume(ctx, req.VolumeID)
    if err != nil {
        return nil, fmt.Errorf("load volume: %w", err)
    }

    // 2. Check if already mounted
    if _, exists := vm.mounts.Load(req.VolumeID); exists {
        return nil, fmt.Errorf("volume %s already mounted", req.VolumeID)
    }

    // 3. If mounting from snapshot, adjust S3 prefix
    s3Prefix := volume.JuiceFS.S3Prefix
    if req.SnapshotID != "" {
        snapshot, err := vm.loadSnapshot(ctx, req.SnapshotID)
        if err != nil {
            return nil, fmt.Errorf("load snapshot: %w", err)
        }
        s3Prefix = snapshot.S3Path
    }

    // 4. Initialize JuiceFS metadata client (PostgreSQL)
    metaConf := meta.DefaultConf()
    metaConf.MountPoint = req.MountPoint
    metaConf.ReadOnly = req.ReadOnly || volume.ReadOnly
    metaConf.Subdir = volume.JuiceFS.Subdir
    
    // Connect to PostgreSQL metadata engine
    metaClient := meta.NewClient(volume.JuiceFS.MetaURL, metaConf)
    format, err := metaClient.Load(true)
    if err != nil {
        return nil, fmt.Errorf("load juicefs format from PostgreSQL: %w", err)
    }

    // 5. Initialize JuiceFS object storage
    format.Bucket = volume.JuiceFS.S3Bucket
    format.Storage = "s3"
    blob, err := vm.createS3Storage(format, s3Prefix)
    if err != nil {
        return nil, fmt.Errorf("create s3 storage: %w", err)
    }

    // 6. Initialize JuiceFS chunk store (with local cache)
    chunkConf := &chunk.Config{
        CacheDir:   filepath.Join(vm.cacheRoot, req.VolumeID),
        CacheSize:  vm.parseCacheSize(volume.JuiceFS.CacheSize),
        BlockSize:  format.BlockSize * 1024,
        Compress:   format.Compression,
        Prefetch:   volume.JuiceFS.Prefetch,
        BufferSize: vm.parseBufferSize(volume.JuiceFS.BufferSize),
        Writeback:  volume.JuiceFS.Writeback,
    }
    chunkStore := chunk.NewCachedStore(blob, *chunkConf, prometheus.DefaultRegisterer)

    // 7. Initialize JuiceFS VFS
    vfsConf := &vfs.Config{
        Meta:   metaConf,
        Format: *format,
        Chunk:  chunkConf,
    }
    vfsInstance := vfs.NewVFS(vfsConf, metaClient, chunkStore, prometheus.DefaultRegisterer, prometheus.DefaultRegistry)

    // 8. Start JuiceFS FUSE server in a goroutine
    mountCtx, cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    
    go func() {
        defer close(done)
        
        // Start FUSE server (blocks until unmount)
        err := fuse.Serve(vfsInstance, "", true, false)
        if err != nil {
            log.Printf("JuiceFS FUSE server error: %v", err)
        }
    }()

    // 9. Wait for mount to be ready (check if mount point is accessible)
    if err := vm.waitForMount(req.MountPoint, 10*time.Second); err != nil {
        cancel()
        return nil, fmt.Errorf("mount not ready: %w", err)
    }

    // 10. Store mount context
    vm.mounts.Store(req.VolumeID, &MountContext{
        VolumeID:   req.VolumeID,
        MountPoint: req.MountPoint,
        vfs:        vfsInstance,
        metaClient: metaClient,
        chunkStore: chunkStore,
        cancel:     cancel,
        done:       done,
        ReadOnly:   metaConf.ReadOnly,
        MountedAt:  time.Now(),
    })

    // 11. Update volume status
    volume.MountCount++
    volume.LastAccessedAt = time.Now()

    return &MountResponse{
        VolumeID:       req.VolumeID,
        MountPoint:     req.MountPoint,
        JuiceFSVersion: version.Version(),
        CacheHit:       vm.hasCachedData(req.VolumeID),
    }, nil
}

// Unmount unmounts a volume
func (vm *VolumeManager) Unmount(ctx context.Context, volumeID string) error {
    // 1. Get mount context
    val, exists := vm.mounts.Load(volumeID)
    if !exists {
        return fmt.Errorf("volume %s not mounted", volumeID)
    }
    mountCtx := val.(*MountContext)

    // 2. Flush all pending writes
    if err := mountCtx.vfs.FlushAll(""); err != nil {
        return fmt.Errorf("flush pending writes: %w", err)
    }

    // 3. Close metadata session
    if err := mountCtx.metaClient.CloseSession(); err != nil {
        log.Printf("close meta session: %v", err)
    }

    // 4. Shutdown object storage client
    object.Shutdown(mountCtx.chunkStore)

    // 5. Cancel FUSE server goroutine
    mountCtx.cancel()

    // 6. Wait for FUSE server to exit
    select {
    case <-mountCtx.done:
    case <-time.After(30 * time.Second):
        return fmt.Errorf("unmount timeout")
    }

    // 7. Remove from active mounts
    vm.mounts.Delete(volumeID)

    return nil
}
```

### 4.3 Snapshot Implementation (S3 Copy)

```go
// CreateSnapshot creates a snapshot via S3 directory copy
func (vm *VolumeManager) CreateSnapshot(ctx context.Context, req *CreateSnapshotRequest) (*Snapshot, error) {
    volume := vm.getVolume(req.VolumeID)
    
    // 1. Flush all pending writes if volume is mounted
    if mountCtx, ok := vm.mounts.Load(req.VolumeID); ok {
        if err := mountCtx.(*MountContext).vfs.FlushAll(""); err != nil {
            return nil, fmt.Errorf("flush pending writes: %w", err)
        }
    }

    // 2. Generate snapshot ID and S3 path
    snapshotID := generateID("snap")
    snapshotS3Path := fmt.Sprintf("snapshots/%s/", snapshotID)

    // 3. Perform S3 copy (copy all objects under volume prefix to snapshot prefix)
    // This is a simple S3 CopyObject operation for each object
    sourcePrefix := volume.JuiceFS.S3Prefix
    targetPrefix := snapshotS3Path
    
    size, fileCount, err := vm.s3CopyDirectory(ctx, volume.JuiceFS.S3Bucket, sourcePrefix, targetPrefix)
    if err != nil {
        return nil, fmt.Errorf("s3 copy: %w", err)
    }

    // 4. Create snapshot metadata
    snapshot := &Snapshot{
        ID:          snapshotID,
        VolumeID:    volume.ID,
        Name:        req.Name,
        Description: req.Description,
        S3Path:      snapshotS3Path,
        Method:      SnapshotMethodS3Copy,
        Metadata:    req.Metadata,
        CreatedAt:   time.Now(),
        SizeBytes:   size,
        FileCount:   fileCount,
    }

    // 5. Store snapshot metadata in PostgreSQL (via Manager)
    if err := vm.saveSnapshot(ctx, snapshot); err != nil {
        return nil, err
    }

    return snapshot, nil
}

// RestoreSnapshot restores from a snapshot (remounts with snapshot S3 path)
func (vm *VolumeManager) RestoreSnapshot(ctx context.Context, req *RestoreSnapshotRequest) error {
    // 1. Load snapshot metadata
    snapshot, err := vm.loadSnapshot(ctx, req.SnapshotID)
    if err != nil {
        return err
    }

    // 2. Unmount current volume if mounted
    if _, mounted := vm.mounts.Load(req.VolumeID); mounted {
        if err := vm.Unmount(ctx, req.VolumeID); err != nil {
            return fmt.Errorf("unmount before restore: %w", err)
        }
    }

    // 3. Update volume's S3 prefix to point to snapshot
    // Option A: Copy snapshot back to volume prefix
    // Option B: Just update volume metadata to use snapshot prefix (faster)
    
    // Here we use Option B (faster, lazy restore)
    volume, err := vm.loadVolume(ctx, req.VolumeID)
    if err != nil {
        return err
    }

    // Backup current S3 prefix
    previousPrefix := volume.JuiceFS.S3Prefix
    
    // Update to snapshot S3 path
    volume.JuiceFS.S3Prefix = snapshot.S3Path
    
    if err := vm.updateVolume(ctx, volume); err != nil {
        return err
    }

    log.Printf("Restored volume %s from snapshot %s (previous: %s, new: %s)",
        req.VolumeID, req.SnapshotID, previousPrefix, snapshot.S3Path)

    return nil
}

// s3CopyDirectory copies all objects from source to target prefix
func (vm *VolumeManager) s3CopyDirectory(ctx context.Context, bucket, sourcePrefix, targetPrefix string) (int64, int32, error) {
    // Use S3 ListObjectsV2 + CopyObject for each object
    // This is simplified; real implementation should handle pagination and parallel copying
    
    s3Client := vm.getS3Client()
    
    listResp, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
        Bucket: aws.String(bucket),
        Prefix: aws.String(sourcePrefix),
    })
    if err != nil {
        return 0, 0, err
    }

    var totalSize int64
    var fileCount int32

    for _, obj := range listResp.Contents {
        sourceKey := *obj.Key
        targetKey := strings.Replace(sourceKey, sourcePrefix, targetPrefix, 1)

        _, err := s3Client.CopyObject(ctx, &s3.CopyObjectInput{
            Bucket:     aws.String(bucket),
            CopySource: aws.String(fmt.Sprintf("%s/%s", bucket, sourceKey)),
            Key:        aws.String(targetKey),
        })
        if err != nil {
            return 0, 0, fmt.Errorf("copy object %s: %w", sourceKey, err)
        }

        totalSize += *obj.Size
        fileCount++
    }

    return totalSize, fileCount, nil
}
```

---

## 五、Performance Optimization Summary

| Operation | Traditional Approach | Sandbox0Volume (JuiceFS) | Improvement |
|-----------|---------------------|--------------------------|-------------|
| **Mount** | OverlayFS + Layer download (seconds) | JuiceFS FUSE mount (~50ms) + lazy load | **100x+** |
| **Snapshot** | tar + S3 upload (seconds to minutes) | S3 CopyObject (~seconds) | **10x+** |
| **Restore** | S3 download + untar (seconds to minutes) | Update S3 prefix pointer (~10ms) | **1000x+** |
| **Cold Start** | 10s-1min | **<100ms** (mount only) | **100x+** |
| **File Access** | Local OverlayFS | JuiceFS local cache + S3 | Similar (with cache) |

### Key Technologies

1. **JuiceFS Embedded Library**: Direct Go library integration, no external process overhead
2. **FUSE Kernel Module**: Zero-copy file operations
3. **Lazy Loading**: Files are fetched from S3 only when accessed
4. **Local Cache**: Hot files cached in `/var/lib/juicefs/cache`
5. **k8s-plugin**: No privileged mode required for `/dev/fuse` access

---

## 六、Complete Flow (<100ms Cold Start)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│              Complete Cold Start Flow (<100ms)                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. User requests to create Sandbox                                         │
│     POST /api/v1/sandboxes/claim                                            │
│     {                                                                       │
│         "template_id": "python-dev",                                        │
│         "volume_config": {                                                  │
│             "volume_id": "vol-123",                                         │
│             "snapshot_id": "snap-latest"  # Optional                        │
│         }                                                                   │
│     }                                                                       │
│                                                                             │
│  2. Manager claims idle Pool Pod (~10ms)                                    │
│     - Update labels: idle → active                                          │
│     - Pass Volume config to Procd                                           │
│                                                                             │
│  3. Manager calls Procd API (~5ms)                                          │
│     POST /api/v1/volumes/vol-123/mount                                      │
│     {                                                                       │
│         "sandbox_id": "sb-abc",                                            │
│         "mount_point": "/workspace",                                        │
│         "snapshot_id": "snap-latest"                                        │
│     }                                                                       │
│                                                                             │
│  4. Procd VolumeManager processes (~50ms)                                   │
│     a. Load Volume metadata from Manager                                   │
│     b. Initialize JuiceFS meta client (Redis connection)                   │
│     c. Initialize JuiceFS chunk store (S3 client)                          │
│     d. Initialize JuiceFS VFS                                              │
│     e. Start FUSE server in goroutine                                      │
│     f. Wait for mount point to be accessible                               │
│                                                                             │
│  5. Sandbox Ready (~70ms total)                                             │
│     ────────────────────────────────────────────────────────────────────   │
│     User sees empty mount point, files loaded on first access              │
│                                                                             │
│  6. Background async tasks (does not block user)                            │
│     - Lazy Load: Fetch files from S3 when accessed                         │
│     - Cache Warming: Prefetch frequently accessed files                    │
│     - Writeback: Flush writes to S3 (async)                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、Multi-Tenant Data Encryption

### 7.1 Encryption Strategy

JuiceFS supports client-side encryption out of the box:

```go
// Volume with encryption enabled
volume := &Volume{
    JuiceFS: JuiceFSConfig{
        // ... other config ...
    },
    EncryptionKeyID: "kms://aws/key/12345",  // KMS key reference
}

// JuiceFS format includes encryption settings
format := &meta.Format{
    // ... other fields ...
    Encryption: "aes256gcm-rsa",  // Client-side encryption
}
```

### 7.2 Key Management

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Key Management Architecture                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  KMS (AWS KMS / GCP KMS / Vault)                                           │
│       │                                                                     │
│       │ 1. Volume creation generates encryption key                        │
│       ▼                                                                     │
│  Procd VolumeManager                                                        │
│       │                                                                     │
│       │ 2. Fetch key from KMS, pass to JuiceFS                             │
│       ▼                                                                     │
│  JuiceFS Client-Side Encryption                                            │
│       │                                                                     │
│       │ 3. Each volume uses independent AES-256-GCM key                    │
│       │ 4. Data encrypted before upload to S3                              │
│       ▼                                                                     │
│  S3 Encrypted Storage                                                      │
│                                                                             │
│  Security Policy:                                                           │
│  - Keys are cached in Procd memory only                                    │
│  - Keys are cleared on Procd exit                                          │
│  - Support key rotation                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 八、Interaction with Manager

### 8.1 Volume Metadata Storage

Volume metadata is stored in PostgreSQL, managed by Manager.

**Important**: JuiceFS also uses the **same PostgreSQL instance** for its internal metadata (inodes, dentries, chunks). This unifies all metadata storage.

```sql
-- Volumes table (managed by Manager, in sandbox0 database)
CREATE TABLE volumes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    team_id TEXT NOT NULL,
    juicefs_config JSONB NOT NULL,  -- JuiceFSConfig as JSON
    capacity TEXT NOT NULL,
    encryption_key_id TEXT,
    read_only BOOLEAN DEFAULT FALSE,
    allowed_sandbox_ids TEXT[],
    tags TEXT[],
    metadata JSONB,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    last_accessed_at TIMESTAMP,
    size_bytes BIGINT DEFAULT 0,
    file_count INTEGER DEFAULT 0,
    mount_count INTEGER DEFAULT 0
);

-- JuiceFS internal tables (created automatically by JuiceFS in the same database)
-- These are managed by JuiceFS, not by Manager:
-- - jfs_node (inodes)
-- - jfs_edge (dentries/directory entries)
-- - jfs_chunk (chunk references)
-- - jfs_symlink (symbolic links)
-- - jfs_xattr (extended attributes)
-- - jfs_session (active sessions)
-- - etc.
```

### 8.2 Procd Call Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Manager → Procd Volume Mount Flow                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Manager fetches Volume metadata from PostgreSQL (sandbox0 database)    │
│     SELECT * FROM volumes WHERE id = 'vol-123';                             │
│                                                                             │
│  2. Manager calls Procd API to mount Volume                                │
│     POST /api/v1/volumes/vol-123/mount                                      │
│     {                                                                       │
│         "sandbox_id": "sb-abc",                                            │
│         "mount_point": "/workspace",                                        │
│         "juicefs_config": {                                                │
│             "meta_url": "postgres://postgres:5432/sandbox0",               │
│             "s3_bucket": "sandbox0-volumes",                               │
│             "s3_prefix": "teams/team-456/volumes/vol-123",                 │
│             "cache_size": "10Gi"                                           │
│         },                                                                 │
│         "snapshot_id": "snap-latest"                                        │
│     }                                                                       │
│                                                                             │
│  3. Procd VolumeManager processes mount                                    │
│     - Initialize JuiceFS meta client (connects to PostgreSQL)              │
│     - Initialize JuiceFS object storage client (S3)                        │
│     - Start FUSE server in goroutine                                       │
│     - Wait for mount point to be accessible                                │
│                                                                             │
│  4. Return mount result                                                    │
│     {                                                                       │
│         "volume_id": "vol-123",                                             │
│         "mount_point": "/workspace",                                        │
│         "juicefs_version": "1.1.0"                                          │
│     }                                                                       │
│                                                                             │
│  Note: JuiceFS automatically creates its internal tables in PostgreSQL     │
│        (jfs_node, jfs_edge, jfs_chunk, etc.) on first mount                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 九、HTTP API (Provided by Procd)

```http
# Mount Volume
POST /api/v1/volumes/{volume_id}/mount
{
    "sandbox_id": "sb-abc123",
    "mount_point": "/workspace",
    "read_only": false,
    "snapshot_id": "snap-001",
    "juicefs_opts": {
        "cache_size": "10Gi",
        "prefetch": 3
    }
}

# Unmount Volume
POST /api/v1/volumes/{volume_id}/unmount

# Get Volume Status
GET /api/v1/volumes/{volume_id}

# Create Snapshot (S3 copy)
POST /api/v1/volumes/{volume_id}/snapshots
{
    "name": "before-deploy",
    "description": "Snapshot before deployment"
}

# List Snapshots
GET /api/v1/volumes/{volume_id}/snapshots

# Restore Snapshot
POST /api/v1/volumes/{volume_id}/restore
{
    "snapshot_id": "snap-001"
}
```

---

## 十、Deployment Configuration

### 10.1 Infrastructure Deployment

#### k8s-plugin DaemonSet

```yaml
# Deploy k8s-plugin as DaemonSet
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: k8s-plugin
  namespace: kube-system
spec:
  selector:
    matchLabels:
      name: k8s-plugin
  template:
    metadata:
      labels:
        name: k8s-plugin
    spec:
      hostNetwork: true
      containers:
      - name: k8s-plugin
        image: ghcr.io/nextflow-io/k8s-plugin:latest
        securityContext:
          privileged: true  # DaemonSet needs privileged to expose /dev/fuse
        volumeMounts:
        - name: device-plugin
          mountPath: /var/lib/kubelet/device-plugins
        - name: dev
          mountPath: /dev
      volumes:
      - name: device-plugin
        hostPath:
          path: /var/lib/kubelet/device-plugins
      - name: dev
        hostPath:
          path: /dev
```

#### PostgreSQL Deployment

```yaml
# Deploy PostgreSQL (unified metadata storage)
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: sandbox0-system
spec:
  ports:
  - port: 5432
    targetPort: 5432
  selector:
    app: postgres
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: sandbox0-system
spec:
  serviceName: postgres
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: sandbox0
        - name: POSTGRES_USER
          value: sandbox0
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: postgres-storage
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 100Gi
```

### 10.2 SandboxTemplate with JuiceFS Support

```yaml
apiVersion: sandbox0.ai/v1alpha1
kind: SandboxTemplate
metadata:
  name: python-dev
spec:
  mainContainer:
    image: sandbox0/procd:latest
    securityContext:
      capabilities:
        add:
          - NET_ADMIN  # For nftables only, no SYS_ADMIN needed
    env:
      - name: SANDBOX_ID
        value: "sb-abc"
      - name: JUICEFS_META_URL
        value: "postgres://sandbox0:password@postgres.sandbox0-system.svc.cluster.local:5432/sandbox0?sslmode=disable"
      - name: JUICEFS_S3_BUCKET
        value: "sandbox0-volumes"
      - name: JUICEFS_S3_REGION
        value: "us-east-1"
      - name: JUICEFS_CACHE_SIZE
        value: "10Gi"
    volumeMounts:
      - name: juicefs-cache
        mountPath: /var/lib/juicefs/cache
    resources:
      limits:
        cpu: "2"
        memory: "4Gi"
        sandbox0.ai/fuse: 1  # Request /dev/fuse access
      requests:
        cpu: "1"
        memory: "2Gi"
  
  volumes:
    - name: juicefs-cache
      emptyDir:
        sizeLimit: 10Gi
  
  pool:
    minIdle: 3
    maxIdle: 10
```

---

## 十一、Comparison with Previous Design

| Aspect | Previous (OverlayFS + Sidecar) | New (JuiceFS Embedded) |
|--------|--------------------------------|------------------------|
| **Architecture** | Procd + S3 Sync Sidecar | Procd only (embedded JuiceFS) |
| **Privileges** | SYS_ADMIN (OverlayFS) + NET_ADMIN | NET_ADMIN only + k8s-plugin |
| **Layer Management** | Complex (Base, Delta, Working layers) | Simple (S3-backed POSIX filesystem) |
| **Snapshot** | Layer delta + S3 upload | S3 directory copy |
| **Cache** | Shared emptyDir between containers | JuiceFS local cache |
| **Mount Time** | ~100ms (OverlayFS + layer download) | ~50ms (FUSE mount + lazy load) |
| **Dependencies** | Sidecar container | None (embedded library) |
| **Complexity** | High (4 files: volume.md with 1200 lines) | Low (1 file, simpler logic) |

---

## 十二、Advantages

1. **No Privileged Container**: Use k8s-plugin to expose `/dev/fuse`, no `privileged: true` required
2. **Simplified Architecture**: No Sidecar, no complex layer management
3. **Production-Ready**: JuiceFS is battle-tested, used in production by many companies
4. **Deep Integration**: Embedded library allows fine-grained control and monitoring
5. **POSIX Compliance**: Full POSIX filesystem semantics (better than object storage)
6. **Efficient Snapshots**: S3 CopyObject is fast and atomic
7. **Multi-Mount Support**: JuiceFS natively supports concurrent mounts (read-only)

---

## 十三、Limitations

1. **PostgreSQL Dependency**: Requires PostgreSQL for metadata (unified with Manager's database)
2. **Cache Warmup**: First file access may be slower (fetching from S3)
3. **Snapshot Size**: Snapshots via S3 copy consume additional S3 storage
4. **FUSE Overhead**: FUSE adds ~10-20% overhead compared to native filesystem
5. **k8s-plugin Requirement**: Requires DaemonSet deployment in cluster

## 十四、Unified PostgreSQL Architecture

All metadata is stored in a single PostgreSQL instance:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    Unified PostgreSQL Metadata Storage                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  PostgreSQL (sandbox0 database)                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  Manager Tables (business metadata)                                   │  │
│  │  ├── volumes - Volume configurations                                  │  │
│  │  ├── snapshots - Snapshot metadata                                    │  │
│  │  ├── teams - Team information                                         │  │
│  │  └── ... - Other business tables                                      │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  JuiceFS Tables (filesystem metadata, auto-created)                   │  │
│  │  ├── jfs_node - Inodes (files, directories)                           │  │
│  │  ├── jfs_edge - Dentries (directory entries)                          │  │
│  │  ├── jfs_chunk - Chunk references (S3 object keys)                    │  │
│  │  ├── jfs_symlink - Symbolic links                                     │  │
│  │  ├── jfs_xattr - Extended attributes                                  │  │
│  │  ├── jfs_session - Active mount sessions                              │  │
│  │  └── ... - Other JuiceFS internal tables                              │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  Benefits:                                                                  │
│  - Single database to manage                                               │
│  - No Redis dependency                                                     │
│  - ACID transactions for metadata operations                               │
│  - Easier backup and recovery                                              │
│  - Better observability (SQL queries for debugging)                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```
