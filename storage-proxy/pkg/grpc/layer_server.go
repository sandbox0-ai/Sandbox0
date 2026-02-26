package grpc

import (
	"context"

	"github.com/sandbox0-ai/infra/pkg/internalauth"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/db"
	"github.com/sandbox0-ai/infra/storage-proxy/pkg/layer"
	pb "github.com/sandbox0-ai/infra/storage-proxy/proto/fs" // LayerService is in the same package
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LayerServer implements the gRPC LayerService
type LayerServer struct {
	pb.UnimplementedLayerServiceServer

	layerMgr *layer.Manager
	repo     *db.Repository
	logger   *logrus.Logger
}

// NewLayerServer creates a new LayerServer
func NewLayerServer(layerMgr *layer.Manager, repo *db.Repository, logger *logrus.Logger) *LayerServer {
	return &LayerServer{
		layerMgr: layerMgr,
		repo:     repo,
		logger:   logger,
	}
}

// ExtractBaseLayer extracts a container image to JuiceFS as a base layer
func (s *LayerServer) ExtractBaseLayer(ctx context.Context, req *pb.ExtractBaseLayerRequest) (*pb.ExtractBaseLayerResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	// Allow system team ID for internal operations
	teamID := req.TeamId
	if teamID == "" {
		teamID = claims.TeamID
	}

	// Convert credentials
	var creds *layer.RegistryCredentials
	if req.Credentials != nil {
		creds = &layer.RegistryCredentials{
			Username:      req.Credentials.Username,
			Password:      req.Credentials.Password,
			ServerAddress: req.Credentials.ServerAddress,
			Auth:          req.Credentials.Auth,
			IdentityToken: req.Credentials.IdentityToken,
			RegistryToken: req.Credentials.RegistryToken,
		}
	}

	// Extract layer
	extractReq := &layer.ExtractLayerRequest{
		ID:          req.Id,
		TeamID:      teamID,
		ImageRef:    req.ImageRef,
		Credentials: creds,
		LayerName:   req.LayerName,
	}

	baseLayer, err := s.layerMgr.ExtractLayer(ctx, extractReq)
	if err != nil {
		if err == layer.ErrLayerExtracting {
			return nil, status.Error(codes.Unavailable, "layer is being extracted")
		}
		s.logger.WithError(err).WithFields(logrus.Fields{
			"team_id":   teamID,
			"image_ref": req.ImageRef,
		}).Error("Failed to extract base layer")
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.ExtractBaseLayerResponse{
		Layer: convertBaseLayer(baseLayer),
	}, nil
}

// GetBaseLayer retrieves base layer information and extraction status
func (s *LayerServer) GetBaseLayer(ctx context.Context, req *pb.GetBaseLayerRequest) (*pb.GetBaseLayerResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	baseLayer, err := s.layerMgr.GetLayer(ctx, req.Id)
	if err != nil {
		if err == layer.ErrLayerNotFound {
			return nil, status.Error(codes.NotFound, "base layer not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetBaseLayerResponse{
		Layer: convertBaseLayer(baseLayer),
	}, nil
}

// GetBaseLayerByImageRef retrieves base layer by team and image reference
func (s *LayerServer) GetBaseLayerByImageRef(ctx context.Context, req *pb.GetBaseLayerByImageRefRequest) (*pb.GetBaseLayerByImageRefResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	// Allow system team ID for internal operations
	teamID := req.TeamId
	if teamID == "" {
		teamID = claims.TeamID
	}

	baseLayer, err := s.layerMgr.GetLayerByImageRef(ctx, teamID, req.ImageRef)
	if err != nil {
		if err == layer.ErrLayerNotFound {
			return nil, status.Error(codes.NotFound, "base layer not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.GetBaseLayerByImageRefResponse{
		Layer: convertBaseLayer(baseLayer),
	}, nil
}

// ListBaseLayers lists all base layers for a team
func (s *LayerServer) ListBaseLayers(ctx context.Context, req *pb.ListBaseLayersRequest) (*pb.ListBaseLayersResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	teamID := req.TeamId
	if teamID == "" {
		teamID = claims.TeamID
	}

	layers, total, err := s.layerMgr.ListLayers(ctx, teamID, req.Status, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var pbLayers []*pb.BaseLayer
	for _, l := range layers {
		pbLayers = append(pbLayers, convertBaseLayer(l))
	}

	return &pb.ListBaseLayersResponse{
		Layers: pbLayers,
		Total:  int32(total),
	}, nil
}

// DeleteBaseLayer deletes a base layer
func (s *LayerServer) DeleteBaseLayer(ctx context.Context, req *pb.DeleteBaseLayerRequest) (*pb.DeleteBaseLayerResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	err := s.layerMgr.DeleteLayer(ctx, req.Id, req.Force)
	if err != nil {
		if err == layer.ErrLayerNotFound {
			return nil, status.Error(codes.NotFound, "base layer not found")
		}
		if err == layer.ErrLayerInUse {
			return nil, status.Error(codes.FailedPrecondition, "base layer is in use")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteBaseLayerResponse{
		Success: true,
		Message: "base layer deleted",
	}, nil
}

// GarbageCollectLayers removes unused base layers
func (s *LayerServer) GarbageCollectLayers(ctx context.Context, req *pb.GarbageCollectLayersRequest) (*pb.GarbageCollectLayersResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	teamID := req.TeamId
	if teamID == "" {
		teamID = claims.TeamID
	}

	deletedIDs, freedBytes, errs := s.layerMgr.GarbageCollect(ctx, teamID, int(req.MinAgeSeconds), int(req.MaxCount), req.DryRun)

	var errStrs []string
	for _, e := range errs {
		errStrs = append(errStrs, e.Error())
	}

	return &pb.GarbageCollectLayersResponse{
		DeletedIds: deletedIDs,
		FreedBytes: freedBytes,
		Errors:     errStrs,
	}, nil
}

// IncrementRefCount increases the reference count for a base layer
func (s *LayerServer) IncrementRefCount(ctx context.Context, req *pb.IncrementRefCountRequest) (*pb.IncrementRefCountResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	newCount, err := s.layerMgr.IncrementRefCount(ctx, req.Id)
	if err != nil {
		if err == layer.ErrLayerNotFound {
			return nil, status.Error(codes.NotFound, "base layer not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.IncrementRefCountResponse{
		NewCount: int32(newCount),
	}, nil
}

// DecrementRefCount decreases the reference count for a base layer
func (s *LayerServer) DecrementRefCount(ctx context.Context, req *pb.DecrementRefCountRequest) (*pb.DecrementRefCountResponse, error) {
	// Validate team ID from context
	claims := internalauth.ClaimsFromContext(ctx)
	if claims == nil || claims.TeamID == "" {
		return nil, status.Error(codes.Unauthenticated, "team id not found in context")
	}

	newCount, err := s.layerMgr.DecrementRefCount(ctx, req.Id)
	if err != nil {
		if err == layer.ErrLayerNotFound {
			return nil, status.Error(codes.NotFound, "base layer not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DecrementRefCountResponse{
		NewCount: int32(newCount),
	}, nil
}

// convertBaseLayer converts db.BaseLayer to protobuf BaseLayer
func convertBaseLayer(l *db.BaseLayer) *pb.BaseLayer {
	if l == nil {
		return nil
	}

	var extractedAt int64
	if l.ExtractedAt != nil {
		extractedAt = l.ExtractedAt.Unix()
	}

	var lastAccessedAt int64
	if l.LastAccessedAt != nil {
		lastAccessedAt = l.LastAccessedAt.Unix()
	}

	var imageDigest string
	if l.ImageDigest != nil {
		imageDigest = *l.ImageDigest
	}

	return &pb.BaseLayer{
		Id:             l.ID,
		TeamId:         l.TeamID,
		ImageRef:       l.ImageRef,
		ImageDigest:    imageDigest,
		LayerPath:      l.LayerPath,
		SizeBytes:      l.SizeBytes,
		Status:         l.Status,
		ExtractedAt:    extractedAt,
		LastError:      l.LastError,
		LastAccessedAt: lastAccessedAt,
		RefCount:       int32(l.RefCount),
		CreatedAt:      l.CreatedAt.Unix(),
		UpdatedAt:      l.UpdatedAt.Unix(),
	}
}
