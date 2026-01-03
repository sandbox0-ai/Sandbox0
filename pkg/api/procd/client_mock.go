package procd

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockProcdClient is a mock implementation of ProcdClient for testing
type MockProcdClient struct {
	mock.Mock
}

// NewMockProcdClient creates a new mock procd client
func NewMockProcdClient() *MockProcdClient {
	return &MockProcdClient{}
}

// Context Management

func (m *MockProcdClient) CreateContext(ctx context.Context, req *CreateContextRequest) (*Context, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Context), args.Error(1)
}

func (m *MockProcdClient) GetContext(ctx context.Context, id string) (*Context, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Context), args.Error(1)
}

func (m *MockProcdClient) ListContexts(ctx context.Context) ([]*Context, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Context), args.Error(1)
}

func (m *MockProcdClient) DeleteContext(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockProcdClient) RestartContext(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockProcdClient) ExecuteCode(ctx context.Context, contextID string, req *ExecuteRequest) (<-chan *ExecuteOutput, error) {
	args := m.Called(ctx, contextID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan *ExecuteOutput), args.Error(1)
}

func (m *MockProcdClient) ExecuteCommand(ctx context.Context, contextID string, req *CommandRequest) error {
	args := m.Called(ctx, contextID, req)
	return args.Error(0)
}

// Volume Management

func (m *MockProcdClient) MountVolume(ctx context.Context, volumeID string, req *VolumeMountRequest) (*VolumeMountResponse, error) {
	args := m.Called(ctx, volumeID, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*VolumeMountResponse), args.Error(1)
}

func (m *MockProcdClient) UnmountVolume(ctx context.Context, volumeID string) error {
	args := m.Called(ctx, volumeID)
	return args.Error(0)
}

func (m *MockProcdClient) GetVolumeStatus(ctx context.Context, volumeID string) (*VolumeStatus, error) {
	args := m.Called(ctx, volumeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*VolumeStatus), args.Error(1)
}

func (m *MockProcdClient) CreateSnapshot(ctx context.Context, req *SnapshotCreateRequest) (*Snapshot, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Snapshot), args.Error(1)
}

func (m *MockProcdClient) ListSnapshots(ctx context.Context, volumeID string) ([]*Snapshot, error) {
	args := m.Called(ctx, volumeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Snapshot), args.Error(1)
}

func (m *MockProcdClient) RestoreSnapshot(ctx context.Context, req *SnapshotRestoreRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockProcdClient) PreloadLayer(ctx context.Context, req *PreloadLayerRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

// Network Policy Management

func (m *MockProcdClient) GetNetworkPolicy(ctx context.Context) (*NetworkPolicy, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*NetworkPolicy), args.Error(1)
}

func (m *MockProcdClient) UpdateNetworkPolicy(ctx context.Context, policy *NetworkPolicy) error {
	args := m.Called(ctx, policy)
	return args.Error(0)
}

func (m *MockProcdClient) ResetNetworkPolicy(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// File Operations

func (m *MockProcdClient) ReadFile(ctx context.Context, path string) (*FileContent, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FileContent), args.Error(1)
}

func (m *MockProcdClient) WriteFile(ctx context.Context, path string, req *FileWriteRequest) error {
	args := m.Called(ctx, path, req)
	return args.Error(0)
}

func (m *MockProcdClient) StatFile(ctx context.Context, path string) (*FileInfo, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FileInfo), args.Error(1)
}

func (m *MockProcdClient) MakeDir(ctx context.Context, path string, req *MakeDirRequest) error {
	args := m.Called(ctx, path, req)
	return args.Error(0)
}

func (m *MockProcdClient) MoveFile(ctx context.Context, req *FileMoveRequest) error {
	args := m.Called(ctx, req)
	return args.Error(0)
}

func (m *MockProcdClient) ListFiles(ctx context.Context, path string) (*FileListResponse, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*FileListResponse), args.Error(1)
}

func (m *MockProcdClient) DeleteFile(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
	return args.Error(0)
}

func (m *MockProcdClient) WatchFiles(ctx context.Context) (<-chan *FileWatchEvent, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(<-chan *FileWatchEvent), args.Error(1)
}

// Health Check

func (m *MockProcdClient) HealthCheck(ctx context.Context) (*HealthResponse, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*HealthResponse), args.Error(1)
}
