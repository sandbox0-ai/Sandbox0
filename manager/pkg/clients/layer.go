package clients

import (
	"context"
	"fmt"
	"time"

	"github.com/sandbox0-ai/infra/manager/pkg/apis/sandbox0/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/sandbox0-ai/infra/storage-proxy/proto/fs"
)

// LayerClient is a client for the LayerService
type LayerClient struct {
	conn   *grpc.ClientConn
	client pb.LayerServiceClient
}

// NewLayerClient creates a new LayerClient
func NewLayerClient(addr string) (*LayerClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create grpc connection: %w", err)
	}

	return &LayerClient{
		conn:   conn,
		client: pb.NewLayerServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *LayerClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// BaseLayerInfo contains base layer information
type BaseLayerInfo struct {
	ID       string
	Status   string
	Error    string
	ImageRef string
}

// EnsureBaseLayer ensures a base layer exists for the given team and image reference.
// If the layer doesn't exist, it triggers extraction.
// This method is used by the template controller to automatically manage baselayers.
func (c *LayerClient) EnsureBaseLayer(ctx context.Context, teamID, imageRef string) (*BaseLayerInfo, error) {
	// First, try to get existing layer by image ref
	getResp, err := c.client.GetBaseLayerByImageRef(ctx, &pb.GetBaseLayerByImageRefRequest{
		TeamId:   teamID,
		ImageRef: imageRef,
	})
	if err == nil && getResp.Layer != nil {
		return &BaseLayerInfo{
			ID:       getResp.Layer.Id,
			Status:   getResp.Layer.Status,
			Error:    getResp.Layer.LastError,
			ImageRef: getResp.Layer.ImageRef,
		}, nil
	}

	// Layer doesn't exist, trigger extraction
	extractResp, err := c.client.ExtractBaseLayer(ctx, &pb.ExtractBaseLayerRequest{
		TeamId:   teamID,
		ImageRef: imageRef,
	})
	if err != nil {
		return nil, fmt.Errorf("extract base layer: %w", err)
	}

	if extractResp.Layer == nil {
		return nil, fmt.Errorf("extraction response has no layer")
	}

	return &BaseLayerInfo{
		ID:       extractResp.Layer.Id,
		Status:   extractResp.Layer.Status,
		Error:    extractResp.Layer.LastError,
		ImageRef: extractResp.Layer.ImageRef,
	}, nil
}

// GetBaseLayer retrieves base layer information by ID
func (c *LayerClient) GetBaseLayer(ctx context.Context, teamID, layerID string) (*BaseLayerInfo, error) {
	resp, err := c.client.GetBaseLayer(ctx, &pb.GetBaseLayerRequest{
		Id:     layerID,
		TeamId: teamID,
	})
	if err != nil {
		return nil, fmt.Errorf("get base layer: %w", err)
	}

	if resp.Layer == nil {
		return nil, fmt.Errorf("layer not found")
	}

	return &BaseLayerInfo{
		ID:       resp.Layer.Id,
		Status:   resp.Layer.Status,
		Error:    resp.Layer.LastError,
		ImageRef: resp.Layer.ImageRef,
	}, nil
}

// IncrementRefCount increments the reference count for a base layer
func (c *LayerClient) IncrementRefCount(ctx context.Context, teamID, layerID string) (int32, error) {
	resp, err := c.client.IncrementRefCount(ctx, &pb.IncrementRefCountRequest{
		Id:     layerID,
		TeamId: teamID,
	})
	if err != nil {
		return 0, fmt.Errorf("increment ref count: %w", err)
	}
	return resp.NewCount, nil
}

// DecrementRefCount decrements the reference count for a base layer
func (c *LayerClient) DecrementRefCount(ctx context.Context, teamID, layerID string) (int32, error) {
	resp, err := c.client.DecrementRefCount(ctx, &pb.DecrementRefCountRequest{
		Id:     layerID,
		TeamId: teamID,
	})
	if err != nil {
		return 0, fmt.Errorf("decrement ref count: %w", err)
	}
	return resp.NewCount, nil
}

// IsLayerReady returns true if the base layer is ready for use
func IsLayerReady(status string) bool {
	return status == v1alpha1.BaseLayerStatusReady
}
