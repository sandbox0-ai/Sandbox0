package grpc

import (
	"context"
	"fmt"
	"io"
	"syscall"
	"time"

	pb "github.com/sandbox0/storage-proxy/proto/fs"
	"github.com/sandbox0/storage-proxy/pkg/audit"
	"github.com/sandbox0/storage-proxy/pkg/auth"
	"github.com/sandbox0/storage-proxy/pkg/volume"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	
	"github.com/juicedata/juicefs/pkg/meta"
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
	// Extract claims for audit logging
	claims, _ := auth.GetClaims(ctx)
	
	// Get volume context
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Get attributes from JuiceFS
	var attr meta.Attr
	inode := meta.Ino(req.Inode)
	err = volCtx.Meta.GetAttr(ctx, inode, &attr)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
		}).Error("GetAttr failed")
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Audit log
	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "getattr",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return convertAttr(&attr), nil
}

// Lookup implements FUSE lookup
func (s *FileSystemServer) Lookup(ctx context.Context, req *pb.LookupRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Lookup entry in JuiceFS
	var inode meta.Ino
	var attr meta.Attr
	parent := meta.Ino(req.Parent)
	err = volCtx.Meta.Lookup(ctx, parent, req.Name, &inode, &attr, true)
	if err != nil {
		if err == syscall.ENOENT {
			return nil, status.Error(codes.NotFound, "entry not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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

// Open implements FUSE open
func (s *FileSystemServer) Open(ctx context.Context, req *pb.OpenRequest) (*pb.OpenResponse, error) {
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)
	var attr meta.Attr
	
	// Open file in JuiceFS (just validation, actual handle managed by client)
	err = volCtx.Meta.GetAttr(ctx, inode, &attr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Generate a handle ID (simple sequential ID)
	handleID := uint64(time.Now().UnixNano())

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "open",
			Inode:     uint64(inode),
			Status:    "success",
		})
	}

	return &pb.OpenResponse{
		HandleId: handleID,
	}, nil
}

// Read implements FUSE read
func (s *FileSystemServer) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	startTime := time.Now()
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Allocate buffer
	buf := make([]byte, req.Size)

	// Read from JuiceFS
	inode := meta.Ino(req.Inode)
	n, err := volCtx.Meta.Read(ctx, inode, uint64(req.Offset), buf)
	
	isEOF := false
	if err != nil && err != io.EOF {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"offset":    req.Offset,
			"size":      req.Size,
		}).Error("Read failed")
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err == io.EOF {
		isEOF = true
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "read",
			Inode:     uint64(inode),
			Size:      int64(n),
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.ReadResponse{
		Data: buf[:n],
		Eof:  isEOF,
	}, nil
}

// Write implements FUSE write
func (s *FileSystemServer) Write(ctx context.Context, req *pb.WriteRequest) (*pb.WriteResponse, error) {
	startTime := time.Now()
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Write to JuiceFS
	inode := meta.Ino(req.Inode)
	err = volCtx.Meta.Write(ctx, inode, uint64(req.Offset), uint32(len(req.Data)), 0, req.Data, time.Now())
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"inode":     req.Inode,
			"offset":    req.Offset,
			"size":      len(req.Data),
		}).Error("Write failed")
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "write",
			Inode:     uint64(inode),
			Size:      int64(len(req.Data)),
			Latency:   time.Since(startTime),
			Status:    "success",
		})
	}

	return &pb.WriteResponse{
		BytesWritten: int64(len(req.Data)),
	}, nil
}

// Create implements FUSE create
func (s *FileSystemServer) Create(ctx context.Context, req *pb.CreateRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create file in JuiceFS
	parent := meta.Ino(req.Parent)
	var inode meta.Ino
	var attr meta.Attr
	
	err = volCtx.Meta.Create(ctx, parent, req.Name, uint16(req.Mode), 0, 0, &inode, &attr)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"volume_id": req.VolumeId,
			"parent":    req.Parent,
			"name":      req.Name,
		}).Error("Create failed")
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
			Operation: "create",
			Inode:     uint64(parent),
			Path:      req.Name,
			Status:    "success",
		})
	}

	return &pb.NodeResponse{
		Inode:      uint64(inode),
		Generation: 0,
		Attr:       convertAttr(&attr),
		HandleId:   uint64(time.Now().UnixNano()),
	}, nil
}

// Mkdir implements FUSE mkdir
func (s *FileSystemServer) Mkdir(ctx context.Context, req *pb.MkdirRequest) (*pb.NodeResponse, error) {
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Create directory in JuiceFS
	parent := meta.Ino(req.Parent)
	var inode meta.Ino
	var attr meta.Attr
	
	err = volCtx.Meta.Mkdir(ctx, parent, req.Name, uint16(req.Mode), 0, 0, &inode, &attr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Unlink file in JuiceFS
	parent := meta.Ino(req.Parent)
	err = volCtx.Meta.Unlink(ctx, parent, req.Name)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Read directory from JuiceFS
	inode := meta.Ino(req.Inode)
	entries, err := volCtx.Meta.Readdir(ctx, inode, uint32(req.Offset), nil)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Convert entries
	var result []*pb.DirEntry
	for _, e := range entries {
		var attr meta.Attr
		volCtx.Meta.GetAttr(ctx, e.Inode, &attr)
		
		result = append(result, &pb.DirEntry{
			Inode:  uint64(e.Inode),
			Offset: 0,
			Name:   string(e.Name),
			Type:   uint32(e.Attr.Typ),
			Attr:   convertAttr(&attr),
		})
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	// Rename in JuiceFS
	oldParent := meta.Ino(req.OldParent)
	newParent := meta.Ino(req.NewParent)
	var inode meta.Ino
	var attr meta.Attr
	
	err = volCtx.Meta.Rename(ctx, oldParent, req.OldName, newParent, req.NewName, uint32(req.Flags), &inode, &attr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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
	claims, _ := auth.GetClaims(ctx)
	
	volCtx, err := s.volMgr.GetVolume(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	inode := meta.Ino(req.Inode)
	var attr meta.Attr
	
	// Set attributes in JuiceFS
	err = volCtx.Meta.SetAttr(ctx, inode, uint16(req.Valid), 0, &attr)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if claims != nil {
		s.auditor.Log(ctx, audit.Event{
			VolumeID:  req.VolumeId,
			SandboxID: claims.SandboxID,
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

// Release implements FUSE release (close)
func (s *FileSystemServer) Release(ctx context.Context, req *pb.ReleaseRequest) (*pb.Empty, error) {
	// Release handle (cleanup if needed)
	return &pb.Empty{}, nil
}

// Helper: convert meta.Attr to protobuf GetAttrResponse
func convertAttr(attr *meta.Attr) *pb.GetAttrResponse {
	return &pb.GetAttrResponse{
		Ino:       uint64(attr.Inode),
		Mode:      uint32(attr.Mode),
		Nlink:     uint32(attr.Nlink),
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

