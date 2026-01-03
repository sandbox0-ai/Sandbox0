package fixtures

import (
	"time"

	"sandbox0.ai/infra/pkg/api/manager"
	"sandbox0.ai/infra/pkg/api/procd"
)

// Procd Fixtures

// NewCreateContextRequest returns a test create context request
func NewCreateContextRequest() *procd.CreateContextRequest {
	return &procd.CreateContextRequest{
		Type:     procd.ProcessTypeREPL,
		Language: "python",
		CWD:      "/home/user",
		EnvVars:  map[string]string{"API_KEY": "test-key"},
	}
}

// NewContext returns a test context
func NewContext() *procd.Context {
	return &procd.Context{
		ID:       "ctx-test-123",
		Type:     procd.ProcessTypeREPL,
		Language: "python",
		CWD:      "/home/user",
		MainProcess: procd.MainProcess{
			ID:   "proc-test-123",
			PID:  1234,
			Type: procd.ProcessTypeREPL,
		},
		CreatedAt: time.Now(),
	}
}

// NewVolumeMountRequest returns a test volume mount request
func NewVolumeMountRequest() *procd.VolumeMountRequest {
	return &procd.VolumeMountRequest{
		SandboxID:  "sb-test-123",
		MountPoint: "/workspace",
		ReadOnly:   false,
		WarmupConfig: &procd.WarmupConfig{
			Enabled:      true,
			BaseLayerIDs: []string{"base-python-3.11"},
		},
	}
}

// NewNetworkPolicy returns a test network policy
func NewNetworkPolicy() *procd.NetworkPolicy {
	return &procd.NetworkPolicy{
		Mode: procd.NetworkModeWhitelist,
		Egress: &procd.NetworkEgressPolicy{
			AllowCIDRs:    []string{"8.8.8.8", "1.1.1.0/24"},
			AllowDomains:  []string{"google.com", "*.github.com"},
			DenyCIDRs:     []string{"10.0.0.0/8"},
			TCPPort:       1080,
		},
	}
}

// NewExecuteRequest returns a test execute request
func NewExecuteRequest() *procd.ExecuteRequest {
	return &procd.ExecuteRequest{
		Code: "x = 100\nprint(x)",
	}
}

// NewFileWriteRequest returns a test file write request
func NewFileWriteRequest() *procd.FileWriteRequest {
	return &procd.FileWriteRequest{
		Content: "SGVsbG8gV29ybGQ=", // base64 encoded "Hello World"
		Mode:    "0644",
	}
}

// NewFileContent returns a test file content
func NewFileContent() *procd.FileContent {
	return &procd.FileContent{
		Content: "SGVsbG8gV29ybGQ=",
		Size:    11,
		Mode:    "0644",
		ModTime: time.Now(),
	}
}

// NewFileInfo returns a test file info
func NewFileInfo() *procd.FileInfo {
	return &procd.FileInfo{
		Name:    "test.txt",
		Path:    "/workspace/test.txt",
		Type:    "file",
		Size:    1024,
		Mode:    "0644",
		ModTime: time.Now(),
		IsLink:  false,
	}
}

// Manager Fixtures

// NewClaimSandboxRequest returns a test claim sandbox request
func NewClaimSandboxRequest() *manager.ClaimSandboxRequest {
	return &manager.ClaimSandboxRequest{
		TemplateID: "python-dev",
		TeamID:     "team-test-123",
		UserID:     "user-test-456",
		SandboxID:  "sb-test-123",
		Config:     NewSandboxConfig(),
	}
}

// NewSandboxConfig returns a test sandbox config
func NewSandboxConfig() *manager.SandboxConfig {
	return &manager.SandboxConfig{
		EnvVars: map[string]string{"API_KEY": "test-key"},
		TTL:     3600,
		Network: &manager.NetworkPolicy{
			Mode: "whitelist",
			Egress: &manager.NetworkEgressPolicy{
				AllowCIDRs:    []string{"8.8.8.8"},
				AllowDomains:  []string{"api.example.com"},
			},
		},
	}
}

// NewSandbox returns a test sandbox
func NewSandbox() *manager.Sandbox {
	now := time.Now()
	expiresAt := now.Add(time.Hour)
	return &manager.Sandbox{
		ID:           "sb-test-123",
		TemplateID:   "python-dev",
		TeamID:       "team-test-123",
		UserID:       "user-test-456",
		Status:       manager.SandboxStatusRunning,
		ProcdAddress: "sb-test-123-pod.default.svc.cluster.local:8080",
		Config:       NewSandboxConfig(),
		CreatedAt:    now,
		ExpiresAt:    &expiresAt,
	}
}

// NewClaimSandboxResponse returns a test claim sandbox response
func NewClaimSandboxResponse() *manager.ClaimSandboxResponse {
	return &manager.ClaimSandboxResponse{
		SandboxID:    "sb-test-123",
		TemplateID:   "python-dev",
		Status:       manager.SandboxStatusStarting,
		ProcdAddress: "sb-test-123-pod.default.svc.cluster.local:8080",
	}
}

// NewTemplateSpec returns a test template spec
func NewTemplateSpec() *manager.TemplateSpec {
	return &manager.TemplateSpec{
		Description: "Python development environment",
		MainContainer: &manager.ContainerSpec{
			Image:   "python:3.11-slim",
			Command: []string{"/procd"},
			EnvVars: map[string]string{"PYTHON_VERSION": "3.11"},
		},
		Resources: &manager.ResourceQuota{
			CPU:    "1",
			Memory: "1Gi",
		},
		Pool: &manager.PoolStrategy{
			MinIdle: 2,
			MaxIdle: 5,
			MaxPool: 10,
		},
	}
}

// NewTemplate returns a test template
func NewTemplate() *manager.Template {
	now := time.Now()
	return &manager.Template{
		ID:          "python-dev",
		Name:        "python-dev",
		Description: "Python development environment",
		Spec:        NewTemplateSpec(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NewCreateTemplateRequest returns a test create template request
func NewCreateTemplateRequest() *manager.CreateTemplateRequest {
	return &manager.CreateTemplateRequest{
		Name: "python-dev",
		Spec: NewTemplateSpec(),
	}
}

// NewVolume returns a test volume
func NewVolume() *manager.Volume {
	return &manager.Volume{
		ID:     "vol-test-123",
		TeamID: "team-test-123",
		Name:   "test-volume",
		Config: &manager.VolumeConfig{
			SizeGB:       10,
			StorageClass: "standard",
		},
	}
}

// NewCreateVolumeRequest returns a test create volume request
func NewCreateVolumeRequest() *manager.CreateVolumeRequest {
	return &manager.CreateVolumeRequest{
		TeamID: "team-test-123",
		Name:   "test-volume",
		Config: &manager.VolumeConfig{
			SizeGB:       10,
			StorageClass: "standard",
		},
	}
}
