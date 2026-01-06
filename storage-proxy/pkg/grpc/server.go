package grpc

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/audit"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/volume"
	pb "github.com/sandbox0-ai/infra/storage-proxy/proto/fs"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/juicedata/juicefs/pkg/meta"
	"github.com/juicedata/juicefs/pkg/vfs"
)

// FileSystemServer implements the gRPC FileSystem service
type FileSystemServer struct {
	pb.UnimplementedFileSystemServer

	volMgr  *volume.Manager
	auditor *audit.Logger
	logger  *logrus.Logger
}

// NewFileSystemServer creates a new file system server
func NewFileSystemServer(volMgr *volume.Manager, auditor *audit.Logger, logger *logrus.Logger) *FileSystemServer {
	return &FileSystemServer{
		volMgr:  volMgr,
		auditor: auditor,
		logger:  logger,
	}
}

// getAuditInfo extracts audit information from context claims
// In internalauth, UserID represents the sandbox ID making the request
func getAuditInfo(ctx context.Context) (sandboxID, teamID string) {
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil {
		return "", ""
	}
	// In storage-proxy context, UserID represents the SandboxID
	return claims.UserID, claims.TeamID
}

// MountVolume mounts a volume
func (s *FileSystemServer) MountVolume(ctx context.Context, req *pb.MountVolumeRequest) (*pb.MountVolumeResponse, error) {
	config := &volume.VolumeConfig{
		MetaURL:        req.Config.MetaUrl,
		S3Bucket:       req.Config.S3Bucket,
		S3Prefix:       req.Config.S3Prefix,
		S3Region:       req.Config.S3Region,
		S3Endpoint:     req.Config.S3Endpoint,
		S3AccessKey:    req.Config.S3AccessKey,
		S3SecretKey:    req.Config.S3SecretKey,
		S3SessionToken: req.Config.S3SessionToken,
		CacheDir:       req.Config.CacheDir,
		CacheSize:      req.Config.CacheSize,
		Prefetch:       int(req.Config.Prefetch),
		BufferSize:     req.Config.BufferSize,
		Writeback:      req.Config.Writeback,
		ReadOnly:       req.Config.ReadOnly,
	}

	err := s.volMgr.MountVolume(ctx, req.VolumeId, config)
	if err != nil {
		s.logger.WithError(err).WithField("volume_id", req.VolumeId).Error("Failed to mount volume")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.MountVolumeResponse{
		VolumeId:  req.VolumeId,
		MountedAt: time.Now().Unix(),
	}, nil
}

// UnmountVolume unmounts a volume
func (s *FileSystemServer) UnmountVolume(ctx context.Context, req *pb.UnmountVolumeRequest) (*pb.Empty, error) {
	err := s.volMgr.UnmountVolume(ctx, req.VolumeId)
	if err != nil {
		s.logger.WithError(err).WithField("volume_id", req.VolumeId).Error("Failed to unmount volume")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.Empty{}, nil
}

// GetAttr implements FUSE getattr
func (s *FileSystemServer) GetAttr(ctx context.Context, req *pb.GetAttrRequest) (*pb.GetAttrResponse, error) {
	// Extract audit info from context
	sandboxID, teamID := getAuditInfo(ctx)

	// Get volume context
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Get attributes from JuiceFS
	var attr meta.Attr
	inode := meta.Ino(req.Inode)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.GetAttr(vfsCtx, inode, &attr)
	if st != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"error":     st,
		}).Error("GetAttr failed")
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	// Audit log
	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "getattr",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return convertAttr(&attr), nil
}

// Lookup implements FUSE lookup
func (s *FileSystemServer) Lookup(ctx context.Context, req *pb.LookupRequest) (*pb.NodeResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Lookup entry in JuiceFS
	var inode meta.Ino
	var attr meta.Attr
	parent := meta.Ino(req.Parent)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Lookup(vfsCtx, parent, req.Name, &inode, &attr, true)
	if st != 0 {
		if st == syscall.ENOENT {
			return nil, status.Error(codes.NotFound, "entry not found")
		}
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "lookup",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
	}, nil
}

// Open implements FUSE open using JuiceFS VFS layer
func (s *FileSystemServer) Open(ctx context.Context, req *pb.OpenRequest) (*pb.OpenResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)

	// Open file using VFS (which creates proper handle with reader/writer)
	vfsCtx := vfs.NewLogContext(meta.Background())

	// VFS.Open returns (Entry, handleID, errno)
	entry, handleID, errno := volCtx.VFS.Open(vfsCtx, inode, req.Flags)
	if errno != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"flags":     req.Flags,
			"error":     errno,
		}).Error("Open failed")
		return nil, status.Error(codes.Internal, syscall.Errno(errno).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "open",
			Inode:     uint64(entry.Inode),
			Status:    "success",
		})
	}

	return &pb.OpenResponse{
		HandleId: handleID,
	}, nil
}

// Read implements FUSE read using JuiceFS VFS layer
func (s *FileSystemServer) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	startTime := time.Now()
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Allocate buffer for read
	buf := make([]byte, req.Size)

	// Create VFS context
	vfsCtx := vfs.NewLogContext(meta.Background())

	// Read from JuiceFS VFS (convert offset to uint64)
	n, errno := volCtx.VFS.Read(vfsCtx, meta.Ino(req.Inode), buf, uint64(req.Offset), req.HandleId)
	if errno != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"offset":    req.Offset,
			"size":      req.Size,
			"handle_id": req.HandleId,
			"error":     errno,
		}).Error("Read failed")

		if sandboxID != "" {
			s.auditor.Log(ctx, audit.Event{
				VolumeID:  req.VolumeId,
				SandboxID: sandboxID,
				TeamID:    teamID,
				Operation: "read",
				Inode:     req.Inode,
				Size:      0,
				Latency:   time.Since(startTime),
				Status:    "error",
			})
		}
		return nil, status.Error(codes.Internal, syscall.Errno(errno).Error())
	}

	// Check if EOF
	eof := false
	if n < len(buf) {
		eof = true
		buf = buf[:n]
	}

	// Audit log
	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "read",
			Inode:     req.Inode,
			Size:      int64(n),
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.ReadResponse{
		Data: buf,
		Eof:  eof,
	}, nil
}

// Write implements FUSE write using JuiceFS VFS layer
func (s *FileSystemServer) Write(ctx context.Context, req *pb.WriteRequest) (*pb.WriteResponse, error) {
	startTime := time.Now()
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create VFS context
	vfsCtx := vfs.NewLogContext(meta.Background())

	// Write to JuiceFS VFS (convert offset to uint64)
	errno := volCtx.VFS.Write(vfsCtx, meta.Ino(req.Inode), req.Data, uint64(req.Offset), req.HandleId)
	if errno != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"offset":    req.Offset,
			"size":      len(req.Data),
			"handle_id": req.HandleId,
			"error":     errno,
		}).Error("Write failed")

		if sandboxID != "" {
			s.auditor.Log(ctx, audit.Event{
				VolumeID:  req.VolumeId,
				SandboxID: sandboxID,
				TeamID:    teamID,
				Operation: "write",
				Inode:     req.Inode,
				Size:      0,
				Latency:   time.Since(startTime),
				Status:    "error",
			})
		}
		return nil, status.Error(codes.Internal, syscall.Errno(errno).Error())
	}

	// Audit log
	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "write",
			Inode:     req.Inode,
			Size:      int64(len(req.Data)),
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.WriteResponse{
		BytesWritten: int64(len(req.Data)),
	}, nil
}

// Create implements FUSE create using JuiceFS VFS layer
func (s *FileSystemServer) Create(ctx context.Context, req *pb.CreateRequest) (*pb.NodeResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create file using VFS (which creates proper handle with reader/writer)
	parent := meta.Ino(req.Parent)
	vfsCtx := vfs.NewLogContext(meta.Background())

	// VFS.Create returns (Entry, handleID, errno)
	entry, handleID, errno := volCtx.VFS.Create(vfsCtx, parent, req.Name, uint16(req.Mode), 0, req.Flags)
	if errno != 0 {
		s.logger.WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"parent":    req.Parent,
			"name":      req.Name,
			"mode":      req.Mode,
			"error":     errno,
		}).Error("Create failed")
		return nil, status.Error(codes.Internal, syscall.Errno(errno).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "create",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(entry.Inode),
		Generation: 0,
		Attr:       convertAttr(entry.Attr),
		HandleId:   handleID,
	}, nil
}

// Mkdir implements FUSE mkdir
func (s *FileSystemServer) Mkdir(ctx context.Context, req *pb.MkdirRequest) (*pb.NodeResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create directory in JuiceFS
	parent := meta.Ino(req.Parent)
	var inode meta.Ino
	var attr meta.Attr

	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Mkdir(vfsCtx, parent, req.Name, uint16(req.Mode), 0, 0, &inode, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "mkdir",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
	}, nil
}

// Unlink implements FUSE unlink
func (s *FileSystemServer) Unlink(ctx context.Context, req *pb.UnlinkRequest) (*pb.Empty, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Unlink file in JuiceFS
	parent := meta.Ino(req.Parent)
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Unlink(vfsCtx, parent, req.Name)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "unlink",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.Empty{}, nil
}

// ReadDir implements FUSE readdir
func (s *FileSystemServer) ReadDir(ctx context.Context, req *pb.ReadDirRequest) (*pb.ReadDirResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Read directory from JuiceFS
	inode := meta.Ino(req.Inode)
	var entries []*meta.Entry
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Readdir(vfsCtx, inode, 1, &entries)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	// Convert entries
	var result []*pb.DirEntry
	for _, e := range entries {
		result = append(result, &pb.DirEntry{
			Inode:  uint64(e.Inode),
			Offset: 0,
			Name:   string(e.Name),
			Type:   uint32(e.Attr.Typ),
			Attr:   convertAttr(e.Attr),
		})
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "readdir",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.ReadDirResponse{
		Entries: result,
		Eof:     false,
	}, nil
}

// Rename implements FUSE rename
func (s *FileSystemServer) Rename(ctx context.Context, req *pb.RenameRequest) (*pb.Empty, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Rename in JuiceFS
	oldParent := meta.Ino(req.OldParent)
	newParent := meta.Ino(req.NewParent)
	var inode meta.Ino
	var attr meta.Attr

	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.Rename(vfsCtx, oldParent, req.OldName, newParent, req.NewName, req.Flags, &inode, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "rename",
			Inode:     uint64(oldParent),
			Path:      fmt.Sprintf("%s -> %s", req.OldName, req.NewName),
			Status:    "success",
		})
	}

	return &pb.Empty{}, nil
}

// SetAttr implements FUSE setattr
func (s *FileSystemServer) SetAttr(ctx context.Context, req *pb.SetAttrRequest) (*pb.SetAttrResponse, error) {
	sandboxID, teamID := getAuditInfo(ctx)

	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)
	var attr meta.Attr

	// Set attributes in JuiceFS
	vfsCtx := vfs.NewLogContext(meta.Background())
	st := volCtx.Meta.SetAttr(vfsCtx, inode, uint16(req.Valid), 0, &attr)
	if st != 0 {
		return nil, status.Error(codes.Internal, syscall.Errno(st).Error())
	}

	if sandboxID != "" {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: sandboxID,
			TeamID:    teamID,
			Operation: "setattr",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.SetAttrResponse{
		Attr: convertAttr(&attr),
	}, nil
}

// Flush implements FUSE flush
func (s *FileSystemServer) Flush(ctx context.Context, req *pb.FlushRequest) (*pb.Empty, error) {
	// Flush is mostly a no-op in JuiceFS (writes are buffered)
	return &pb.Empty{}, nil
}

// Fsync implements FUSE fsync
func (s *FileSystemServer) Fsync(ctx context.Context, req *pb.FsyncRequest) (*pb.Empty, error) {
	// Fsync - data is synced by chunk store's writeback cache
	return &pb.Empty{}, nil
}

// Release implements FUSE release (close) using JuiceFS VFS layer
func (s *FileSystemServer) Release(ctx context.Context, req *pb.ReleaseRequest) (*pb.Empty, error) {
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Release the file handle in VFS
	vfsCtx := vfs.NewLogContext(meta.Background())
	volCtx.VFS.Release(vfsCtx, meta.Ino(req.Inode), req.HandleId)

	s.logger.WithFields(logrus.Fields{
		"volume_id": req.VolumeId,
		"inode":     req.Inode,
		"handle_id": req.HandleId,
	}).Debug("Released file handle")

	return &pb.Empty{}, nil
}

// Helper: convert meta.Attr to protobuf GetAttrResponse
func convertAttr(attr *meta.Attr) *pb.GetAttrResponse {
	return &pb.GetAttrResponse{
		Ino:       uint64(meta.RootInode), // Would need proper inode tracking
		Mode:      uint32(attr.Mode),
		Nlink:     attr.Nlink,
		Uid:       attr.Uid,
		Gid:       attr.Gid,
		Rdev:      uint64(attr.Rdev),
		Size:      attr.Length,
		Blocks:    0,
		AtimeSec:  attr.Atime,
		AtimeNsec: int64(attr.Atimensec),
		MtimeSec:  attr.Mtime,
		MtimeNsec: int64(attr.Mtimensec),
		CtimeSec:  attr.Ctime,
		CtimeNsec: int64(attr.Ctimensec),
	}
}
