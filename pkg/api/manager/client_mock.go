package manager

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockManagerClient is a mock implementation of ManagerClient for testing
type MockManagerClient struct {
	mock.Mock
}

// NewMockManagerClient creates a new mock manager client
func NewMockManagerClient() *MockManagerClient {
	return &MockManagerClient{}
}

// Sandbox Management

func (m *MockManagerClient) ClaimSandbox(ctx context.Context, req *ClaimSandboxRequest) (*ClaimSandboxResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ClaimSandboxResponse), args.Error(1)
}

func (m *MockManagerClient) GetSandbox(ctx context.Context, id string) (*Sandbox, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Sandbox), args.Error(1)
}

func (m *MockManagerClient) ListSandboxes(ctx context.Context, teamID, userID string) ([]*Sandbox, error) {
	args := m.Called(ctx, teamID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Sandbox), args.Error(1)
}

func (m *MockManagerClient) GetSandboxStatus(ctx context.Context, id string) (*SandboxStatusResponse, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SandboxStatusResponse), args.Error(1)
}

func (m *MockManagerClient) UpdateSandbox(ctx context.Context, id string, req *UpdateSandboxRequest) error {
	args := m.Called(ctx, id, req)
	return args.Error(0)
}

func (m *MockManagerClient) DeleteSandbox(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockManagerClient) PauseSandbox(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockManagerClient) ResumeSandbox(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockManagerClient) RefreshSandbox(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// Template Management

func (m *MockManagerClient) CreateTemplate(ctx context.Context, req *CreateTemplateRequest) (*Template, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Template), args.Error(1)
}

func (m *MockManagerClient) GetTemplate(ctx context.Context, id string) (*Template, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Template), args.Error(1)
}

func (m *MockManagerClient) ListTemplates(ctx context.Context) ([]*Template, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Template), args.Error(1)
}

func (m *MockManagerClient) UpdateTemplate(ctx context.Context, id string, req *UpdateTemplateRequest) error {
	args := m.Called(ctx, id, req)
	return args.Error(0)
}

func (m *MockManagerClient) DeleteTemplate(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockManagerClient) WarmPool(ctx context.Context, id string, req *WarmPoolRequest) (*WarmPoolResponse, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*WarmPoolResponse), args.Error(1)
}

// Volume Management

func (m *MockManagerClient) CreateVolume(ctx context.Context, req *CreateVolumeRequest) (*Volume, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Volume), args.Error(1)
}

func (m *MockManagerClient) GetVolume(ctx context.Context, id string) (*Volume, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Volume), args.Error(1)
}

func (m *MockManagerClient) ListVolumes(ctx context.Context, teamID string) ([]*Volume, error) {
	args := m.Called(ctx, teamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*Volume), args.Error(1)
}

func (m *MockManagerClient) DeleteVolume(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
